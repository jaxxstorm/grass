package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	randv2 "math/rand/v2"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
	"github.com/jaxxstorm/grass/search"
)

const JournalPartition = "__grass_archive_journal__"
const JournalVersion = 1

type JournalState struct {
	Epoch    string
	Sequence int64
}
type JournalEvent struct {
	Sequence               int64
	Platform, URL, WriteID string
}

func JournalKey(sequence int64) string { return fmt.Sprintf("event#%020d", sequence) }
func journalID() string                { var b [16]byte; rand.Read(b[:]); return hex.EncodeToString(b[:]) }
func attrString(item map[string]types.AttributeValue, k string) string {
	v, _ := item[k].(*types.AttributeValueMemberS)
	if v == nil {
		return ""
	}
	return v.Value
}
func attrInt(item map[string]types.AttributeValue, k string) (int64, error) {
	v, ok := item[k].(*types.AttributeValueMemberN)
	if !ok {
		return 0, fmt.Errorf("missing numeric %s", k)
	}
	return strconv.ParseInt(v.Value, 10, 64)
}
func itemKey(platform, url string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"Platform": &types.AttributeValueMemberS{Value: platform}, "SortKey": &types.AttributeValueMemberS{Value: url}}
}
func DecodeJournalState(item map[string]types.AttributeValue) (JournalState, error) {
	if len(item) == 0 {
		return JournalState{}, nil
	}
	v, err := attrInt(item, "FormatVersion")
	if err != nil || v != JournalVersion {
		return JournalState{}, errors.New("unsupported archive journal version")
	}
	s, err := attrInt(item, "Sequence")
	epoch := attrString(item, "Epoch")
	if err != nil || s < 0 || epoch == "" {
		return JournalState{}, errors.New("invalid archive journal state")
	}
	return JournalState{epoch, s}, nil
}
func DecodeJournalEvent(item map[string]types.AttributeValue) (JournalEvent, error) {
	seq, err := attrInt(item, "Sequence")
	e := JournalEvent{Sequence: seq, Platform: attrString(item, "ResultPlatform"), URL: attrString(item, "ResultURL"), WriteID: attrString(item, "WriteID")}
	if err != nil || seq < 1 || e.Platform == "" || e.URL == "" || e.WriteID == "" || attrString(item, "SortKey") != JournalKey(seq) {
		return e, errors.New("invalid archive journal event")
	}
	return e, nil
}

type journalWriter interface {
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	TransactWriteItems(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

func saveJournal(ctx context.Context, client journalWriter, table string, r search.SearchResult) error {
	writeID := journalID()
	var last error
	for attempt := 0; attempt < 8; attempt++ {
		control, err := client.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(table), Key: itemKey(JournalPartition, "state"), ConsistentRead: aws.Bool(true)})
		if err != nil {
			return fmt.Errorf("read archive journal: %w", err)
		}
		state, err := DecodeJournalState(control.Item)
		if err != nil {
			return err
		}
		if state.Sequence == math.MaxInt64 {
			return errors.New("archive journal sequence exhausted")
		}
		condition := "attribute_not_exists(Platform)"
		values := map[string]types.AttributeValue{":next": &types.AttributeValueMemberN{Value: strconv.FormatInt(state.Sequence+1, 10)}, ":version": &types.AttributeValueMemberN{Value: "1"}}
		if state.Epoch == "" {
			state.Epoch = journalID()
		} else {
			condition = "#seq=:previous AND Epoch=:epoch AND FormatVersion=:version"
			values[":previous"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(state.Sequence, 10)}
		}
		values[":epoch"] = &types.AttributeValueMemberS{Value: state.Epoch}
		item := resultItem(r)
		item["JournalEpoch"] = &types.AttributeValueMemberS{Value: state.Epoch}
		item["JournalSequence"] = values[":next"]
		item["JournalWriteID"] = &types.AttributeValueMemberS{Value: writeID}
		event := itemKey(JournalPartition, JournalKey(state.Sequence+1))
		event["Sequence"] = values[":next"]
		event["Epoch"] = values[":epoch"]
		event["ResultPlatform"] = &types.AttributeValueMemberS{Value: r.Platform}
		event["ResultURL"] = &types.AttributeValueMemberS{Value: r.URL}
		event["WriteID"] = &types.AttributeValueMemberS{Value: writeID}
		input := &dynamodb.TransactWriteItemsInput{ClientRequestToken: aws.String(journalID()), TransactItems: []types.TransactWriteItem{
			{Put: &types.Put{TableName: aws.String(table), Item: item, ConditionExpression: aws.String("attribute_not_exists(Platform)")}},
			{Update: &types.Update{TableName: aws.String(table), Key: itemKey(JournalPartition, "state"), UpdateExpression: aws.String("SET #seq=:next, Epoch=:epoch, FormatVersion=:version"), ConditionExpression: aws.String(condition), ExpressionAttributeNames: map[string]string{"#seq": "Sequence"}, ExpressionAttributeValues: values}},
			{Put: &types.Put{TableName: aws.String(table), Item: event, ConditionExpression: aws.String("attribute_not_exists(Platform)")}},
		}}
		_, last = client.TransactWriteItems(ctx, input)
		if last == nil {
			return nil
		}
		// Read after any uncertain outcome: the write ID survives all attempts
		// but another writer's insertion must never be treated as our new result.
		stored, readErr := client.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(table), Key: itemKey(r.Platform, r.URL), ConsistentRead: aws.Bool(true)})
		if readErr != nil {
			return fmt.Errorf("resolve journal transaction (%v): %w", last, readErr)
		}
		if len(stored.Item) > 0 {
			if attrString(stored.Item, "JournalWriteID") == writeID {
				return nil
			}
			return ErrAlreadyExists
		}
		var api smithy.APIError
		if errors.As(last, &api) {
			switch api.ErrorCode() {
			case "AccessDeniedException", "ValidationException", "ResourceNotFoundException", "IdempotentParameterMismatchException":
				return last
			}
		}
		delay := time.Duration(min(1000, 25<<attempt)+randv2.IntN(25)) * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return fmt.Errorf("archive journal transaction exhausted retries: %w", last)
}
