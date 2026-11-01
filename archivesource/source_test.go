package archivesource

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/jaxxstorm/grass/storage"
)

type fakeDynamo struct {
	batchCalls    int
	gap, missing  bool
	identityField string
	identityValue types.AttributeValue
}

func (f *fakeDynamo) GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return &dynamodb.GetItemOutput{}, nil
}
func (f *fakeDynamo) Scan(_ context.Context, in *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	if !aws.ToBool(in.ConsistentRead) || in.ReturnConsumedCapacity != types.ReturnConsumedCapacityTotal || aws.ToInt32(in.Limit) > 100 {
		return nil, errors.New("unbounded/inconsistent scan")
	}
	return &dynamodb.ScanOutput{}, nil
}
func (f *fakeDynamo) Query(_ context.Context, in *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	if !aws.ToBool(in.ConsistentRead) || in.ReturnConsumedCapacity != types.ReturnConsumedCapacityTotal {
		return nil, errors.New("inconsistent journal read")
	}
	if f.gap {
		return &dynamodb.QueryOutput{}, nil
	}
	e := key(storage.JournalPartition, storage.JournalKey(1))
	e["Sequence"] = &types.AttributeValueMemberN{Value: "1"}
	e["Epoch"] = &types.AttributeValueMemberS{Value: "epoch"}
	e["ResultPlatform"] = &types.AttributeValueMemberS{Value: "X"}
	e["ResultURL"] = &types.AttributeValueMemberS{Value: "https://example.test"}
	e["WriteID"] = &types.AttributeValueMemberS{Value: "write"}
	return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{e}}, nil
}
func (f *fakeDynamo) BatchGetItem(_ context.Context, in *dynamodb.BatchGetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
	f.batchCalls++
	if !aws.ToBool(in.RequestItems["table"].ConsistentRead) {
		return nil, errors.New("inconsistent record read")
	}
	if f.batchCalls == 1 {
		return &dynamodb.BatchGetItemOutput{UnprocessedKeys: in.RequestItems}, nil
	}
	item := key("X", "https://example.test")
	item["Timestamp"] = &types.AttributeValueMemberN{Value: "1"}
	item["JournalEpoch"] = &types.AttributeValueMemberS{Value: "epoch"}
	item["JournalSequence"] = &types.AttributeValueMemberN{Value: "1"}
	item["JournalWriteID"] = &types.AttributeValueMemberS{Value: "write"}
	if f.identityField != "" {
		item[f.identityField] = f.identityValue
	}
	if f.missing {
		return &dynamodb.BatchGetItemOutput{}, nil
	}
	return &dynamodb.BatchGetItemOutput{Responses: map[string][]map[string]types.AttributeValue{"table": {item}}}, nil
}
func TestDynamoIncrementalAdapter(t *testing.T) {
	f := &fakeDynamo{}
	d := &Dynamo{client: f, table: "table", budget: pacer{rate: 2}}
	if _, err := d.Scan(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	p, err := d.Changes(context.Background(), "epoch", 0, 1)
	if err != nil || len(p.Records) != 1 || p.Sequence != 1 || f.batchCalls != 2 {
		t.Fatalf("replay: %+v %v", p, err)
	}
	f.gap = true
	if _, err = d.Changes(context.Background(), "epoch", 0, 1); err == nil {
		t.Fatal("gap accepted")
	}
	f.gap = false
	f.missing = true
	if _, err = d.Changes(context.Background(), "epoch", 0, 1); err == nil {
		t.Fatal("missing reference accepted")
	}
	pacer := pacer{rate: 2}
	pacer.charge(time.Now(), 100)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if err = pacer.wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("pacer ignored cancellation")
	}
	item := key(storage.JournalPartition, "state")
	item["Timestamp"] = &types.AttributeValueMemberN{Value: "1"}
	if _, ok := decode(item); ok {
		t.Fatal("journal record leaked into results")
	}
}
func TestSQLiteLegacyReadOnly(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "source.db")
	if _, err := NewSQLite(ctx, path); err == nil {
		t.Fatal("missing source created")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE search_results(Platform TEXT,URL TEXT,Keyword TEXT,Title TEXT,Timestamp INTEGER); INSERT INTO search_results VALUES('X','https://example.test','grass','legacy',1)`)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err := s.Scan(ctx, "")
	if err != nil || len(p.Records) != 1 || p.Records[0].Content != "" {
		t.Fatalf("legacy import: %+v %v", p, err)
	}
	if _, err = db.Exec("SELECT Content FROM search_results"); err == nil {
		t.Fatal("source schema was migrated")
	}
}

func TestConsumedCapacityBudget(t *testing.T) {
	p := pacer{rate: 2}
	start := time.Now()
	p.charge(start, 6)
	if p.next.Sub(start) != 3*time.Second {
		t.Fatal("actual capacity was not charged")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(p.wait(ctx), context.Canceled) {
		t.Fatal("capacity wait ignored cancellation")
	}
}
