package archivesource

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/jaxxstorm/grass/search"
	"github.com/jaxxstorm/grass/storage"
)

type dynamoAPI interface {
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Scan(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	BatchGetItem(context.Context, *dynamodb.BatchGetItemInput, ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error)
}
type Dynamo struct {
	client dynamoAPI
	table  string
	budget pacer
}

func NewDynamo(ctx context.Context, table string, limits storage.ArchiveLimits) (*Dynamo, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return NewDynamoFromConfig(cfg, table, limits)
}

// NewDynamoFromConfig uses the same resolved configuration as the cache identity.
func NewDynamoFromConfig(cfg aws.Config, table string, limits storage.ArchiveLimits) (*Dynamo, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	if cfg.Region == "" {
		return nil, errors.New("AWS region is missing; set AWS_REGION or configure the selected profile")
	}
	return &Dynamo{client: dynamodb.NewFromConfig(cfg), table: table, budget: pacer{rate: limits.ReadCapacity}}, nil
}
func (d *Dynamo) Close() error { return nil }
func key(platform, url string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"Platform": &types.AttributeValueMemberS{Value: platform}, "SortKey": &types.AttributeValueMemberS{Value: url}}
}
func str(item map[string]types.AttributeValue, k string) string {
	v, _ := item[k].(*types.AttributeValueMemberS)
	if v == nil {
		return ""
	}
	return v.Value
}
func units(c *types.ConsumedCapacity) float64 {
	if c == nil {
		return 0
	}
	return aws.ToFloat64(c.CapacityUnits)
}
func (d *Dynamo) Head(ctx context.Context) (Head, error) {
	if err := d.budget.wait(ctx); err != nil {
		return Head{}, err
	}
	start := time.Now()
	out, err := d.client.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(d.table), Key: key(storage.JournalPartition, "state"), ConsistentRead: aws.Bool(true), ReturnConsumedCapacity: types.ReturnConsumedCapacityTotal})
	if err != nil {
		return Head{}, fmt.Errorf("read journal for %s: %w", d.table, err)
	}
	d.budget.charge(start, units(out.ConsumedCapacity))
	s, err := storage.DecodeJournalState(out.Item)
	if err != nil {
		return Head{}, err
	}
	mode := "journal"
	if s.Epoch == "" {
		mode = "legacy"
	}
	return Head{mode, s.Epoch, s.Sequence}, nil
}
func cursorEncode(k map[string]types.AttributeValue) string {
	if len(k) == 0 {
		return ""
	}
	b, _ := json.Marshal(map[string]string{"Platform": str(k, "Platform"), "SortKey": str(k, "SortKey")})
	return base64.RawURLEncoding.EncodeToString(b)
}
func cursorDecode(s string) (map[string]types.AttributeValue, error) {
	if s == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	var v map[string]string
	if err = json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	if v["Platform"] == "" || v["SortKey"] == "" {
		return nil, errors.New("invalid scan checkpoint")
	}
	return key(v["Platform"], v["SortKey"]), nil
}
func decode(item map[string]types.AttributeValue) (storage.ArchiveRecord, bool) {
	n, ok := item["Timestamp"].(*types.AttributeValueMemberN)
	if !ok {
		return storage.ArchiveRecord{}, false
	}
	timestamp, err := strconv.ParseInt(n.Value, 10, 64)
	if err != nil {
		return storage.ArchiveRecord{}, false
	}
	r := storage.ArchiveRecord{SearchResult: search.SearchResult{Platform: str(item, "Platform"), URL: str(item, "SortKey"), Title: str(item, "Title"), Keyword: str(item, "Keyword"), Timestamp: timestamp, Content: str(item, "Content"), AuthorID: str(item, "AuthorID"), AuthorHandle: str(item, "AuthorHandle"), AuthorName: str(item, "AuthorName")}}
	return r, valid(r)
}
func (d *Dynamo) Scan(ctx context.Context, cursor string) (Page, error) {
	startKey, err := cursorDecode(cursor)
	if err != nil {
		return Page{}, err
	}
	if err = d.budget.wait(ctx); err != nil {
		return Page{}, err
	}
	started := time.Now()
	out, err := d.client.Scan(ctx, &dynamodb.ScanInput{TableName: aws.String(d.table), ExclusiveStartKey: startKey, ConsistentRead: aws.Bool(true), Limit: aws.Int32(100), ReturnConsumedCapacity: types.ReturnConsumedCapacityTotal})
	if err != nil {
		return Page{}, fmt.Errorf("scan %s: %w", d.table, err)
	}
	d.budget.charge(started, units(out.ConsumedCapacity))
	p := Page{Cursor: cursorEncode(out.LastEvaluatedKey), Examined: max(int(out.ScannedCount), len(out.Items))}
	for _, item := range out.Items {
		if r, ok := decode(item); ok {
			p.Records = append(p.Records, r)
		}
	}
	return p, nil
}
func (d *Dynamo) Changes(ctx context.Context, epoch string, after, target int64) (Page, error) {
	p := Page{Sequence: after}
	if after >= target {
		return p, nil
	}
	if err := d.budget.wait(ctx); err != nil {
		return p, err
	}
	start := time.Now()
	out, err := d.client.Query(ctx, &dynamodb.QueryInput{TableName: aws.String(d.table), ConsistentRead: aws.Bool(true), Limit: aws.Int32(100), ReturnConsumedCapacity: types.ReturnConsumedCapacityTotal, KeyConditionExpression: aws.String("Platform=:p AND SortKey BETWEEN :from AND :to"), ExpressionAttributeValues: map[string]types.AttributeValue{":p": &types.AttributeValueMemberS{Value: storage.JournalPartition}, ":from": &types.AttributeValueMemberS{Value: storage.JournalKey(after + 1)}, ":to": &types.AttributeValueMemberS{Value: storage.JournalKey(target)}}})
	if err != nil {
		return p, err
	}
	d.budget.charge(start, units(out.ConsumedCapacity))
	if len(out.Items) == 0 {
		return p, fmt.Errorf("missing event: %w", ErrJournal)
	}
	events := make([]storage.JournalEvent, 0, len(out.Items))
	keys := []map[string]types.AttributeValue{}
	seen := map[string]bool{}
	for _, item := range out.Items {
		e, err := storage.DecodeJournalEvent(item)
		if err != nil {
			return p, err
		}
		if e.Sequence != p.Sequence+1 || e.Sequence > target || str(item, "Epoch") != epoch {
			return p, fmt.Errorf("sequence or epoch discontinuity: %w", ErrJournal)
		}
		p.Sequence = e.Sequence
		events = append(events, e)
		identity := e.Platform + "\x00" + e.URL
		if !seen[identity] {
			keys = append(keys, key(e.Platform, e.URL))
			seen[identity] = true
		}
	}
	items := map[string]map[string]types.AttributeValue{}
	// BatchGet bills each record separately. Avoid a 100-RCU burst on a
	// low-capacity table even when the journal Query fits in a small page.
	batchSize := max(1, int(min(100.0, d.budget.rate)))
	for len(keys) > 0 {
		n := min(batchSize, len(keys))
		pending := keys[:n]
		keys = keys[n:]
		for attempt := 0; len(pending) > 0 && attempt < 8; attempt++ {
			if err := d.budget.wait(ctx); err != nil {
				return p, err
			}
			started := time.Now()
			got, err := d.client.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{RequestItems: map[string]types.KeysAndAttributes{d.table: {Keys: pending, ConsistentRead: aws.Bool(true)}}, ReturnConsumedCapacity: types.ReturnConsumedCapacityTotal})
			if err != nil {
				return p, err
			}
			total := 0.0
			for _, c := range got.ConsumedCapacity {
				total += aws.ToFloat64(c.CapacityUnits)
			}
			d.budget.charge(started, total)
			for _, item := range got.Responses[d.table] {
				r, ok := decode(item)
				if !ok {
					return p, errors.New("invalid referenced archive record")
				}
				items[r.Platform+"\x00"+r.URL] = item
			}
			pending = got.UnprocessedKeys[d.table].Keys
			if len(pending) > 0 {
				wait := time.NewTimer(time.Duration(25<<attempt) * time.Millisecond)
				select {
				case <-ctx.Done():
					wait.Stop()
					return p, ctx.Err()
				case <-wait.C:
				}
			}
		}
		if len(pending) > 0 {
			return p, errors.New("DynamoDB left unprocessed journal references after retries")
		}
	}
	for _, e := range events {
		item, ok := items[e.Platform+"\x00"+e.URL]
		if !ok {
			return p, fmt.Errorf("missing referenced record: %w", ErrJournal)
		}
		sequence, ok := item["JournalSequence"].(*types.AttributeValueMemberN)
		if !ok || sequence.Value != strconv.FormatInt(e.Sequence, 10) || str(item, "JournalEpoch") != epoch || str(item, "JournalWriteID") != e.WriteID {
			return Page{}, fmt.Errorf("referenced record write identity changed: %w", ErrJournal)
		}
		r, _ := decode(item)
		p.Records = append(p.Records, r)
	}
	p.Examined = len(events)
	return p, nil
}
