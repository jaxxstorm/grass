package storage

import (
	"context"
	"errors"
	"math"
	"strconv"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
	"github.com/jaxxstorm/grass/search"
)

type memoryJournal struct {
	mu                 sync.Mutex
	items              map[string]map[string]types.AttributeValue
	loseResponse, deny bool
	throttle           bool
}

func journalMapKey(m map[string]types.AttributeValue) string {
	return attrString(m, "Platform") + "/" + attrString(m, "SortKey")
}
func (m *memoryJournal) GetItem(_ context.Context, in *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if in.ConsistentRead == nil || !*in.ConsistentRead {
		return nil, errors.New("journal read must be consistent")
	}
	return &dynamodb.GetItemOutput{Item: m.items[journalMapKey(in.Key)]}, nil
}
func (m *memoryJournal) TransactWriteItems(_ context.Context, in *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(in.TransactItems) != 3 || in.ClientRequestToken == nil {
		return nil, errors.New("invalid atomic transaction")
	}
	if m.deny {
		return nil, &smithy.GenericAPIError{Code: "ValidationException", Message: "injected rejection"}
	}
	if m.throttle {
		m.throttle = false
		return nil, &types.ProvisionedThroughputExceededException{}
	}
	r, u, e := in.TransactItems[0].Put, in.TransactItems[1].Update, in.TransactItems[2].Put
	if r.ConditionExpression == nil || u.ConditionExpression == nil || e.ConditionExpression == nil {
		return nil, errors.New("missing transaction conditions")
	}
	old := m.items[journalMapKey(u.Key)]
	seq, _ := attrInt(old, "Sequence")
	previous, _ := attrInt(map[string]types.AttributeValue{"Sequence": u.ExpressionAttributeValues[":previous"]}, "Sequence")
	if len(m.items[journalMapKey(r.Item)]) > 0 || seq != previous {
		return nil, &types.TransactionCanceledException{}
	}
	control := itemKey(JournalPartition, "state")
	control["Epoch"] = u.ExpressionAttributeValues[":epoch"]
	control["Sequence"] = u.ExpressionAttributeValues[":next"]
	control["FormatVersion"] = u.ExpressionAttributeValues[":version"]
	m.items[journalMapKey(r.Item)] = r.Item
	m.items[journalMapKey(control)] = control
	m.items[journalMapKey(e.Item)] = e.Item
	if m.loseResponse {
		m.loseResponse = false
		return nil, errors.New("connection lost after commit")
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}
func TestJournalConcurrentWritesAndLostResponse(t *testing.T) {
	m := &memoryJournal{items: map[string]map[string]types.AttributeValue{}, loseResponse: true, throttle: true}
	var wg sync.WaitGroup
	for n := 0; n < 4; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			err := saveJournal(context.Background(), m, "table", search.SearchResult{Platform: "X", URL: "https://example.test/" + strconv.Itoa(n)})
			if err != nil {
				t.Error(err)
			}
		}(n)
	}
	wg.Wait()
	state, err := DecodeJournalState(m.items[journalMapKey(itemKey(JournalPartition, "state"))])
	if err != nil || state.Sequence != 4 || len(m.items) != 9 {
		t.Fatalf("non-atomic or duplicate journal: %+v %v items=%d", state, err, len(m.items))
	}
	for seq := int64(1); seq <= 4; seq++ {
		event, err := DecodeJournalEvent(m.items[journalMapKey(itemKey(JournalPartition, JournalKey(seq)))])
		if err != nil || event.Sequence != seq {
			t.Fatalf("event %d: %v", seq, err)
		}
	}
	err = saveJournal(context.Background(), m, "table", search.SearchResult{Platform: "X", URL: "https://example.test/0"})
	if !errors.Is(err, ErrAlreadyExists) || len(m.items) != 9 {
		t.Fatal("duplicate allocated another event")
	}
}
func TestJournalRejectionAndInvalidControl(t *testing.T) {
	m := &memoryJournal{items: map[string]map[string]types.AttributeValue{}, deny: true}
	if err := saveJournal(context.Background(), m, "table", search.SearchResult{Platform: "X", URL: "https://example.test"}); err == nil || len(m.items) != 0 {
		t.Fatal("failed transaction published data")
	}
	for _, tc := range []struct{ version, seq string }{{"2", "0"}, {"1", strconv.FormatInt(math.MaxInt64, 10)}} {
		m.deny = false
		c := itemKey(JournalPartition, "state")
		c["Epoch"] = &types.AttributeValueMemberS{Value: "epoch"}
		c["FormatVersion"] = &types.AttributeValueMemberN{Value: tc.version}
		c["Sequence"] = &types.AttributeValueMemberN{Value: tc.seq}
		m.items[journalMapKey(c)] = c
		if err := saveJournal(context.Background(), m, "table", search.SearchResult{Platform: "X", URL: "https://example.test"}); err == nil {
			t.Fatal("unsupported control accepted")
		}
	}
}
