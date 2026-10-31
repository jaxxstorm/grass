package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type scanFixture struct {
	calls     int
	consumed  float64
	firstCall time.Time
	gap       time.Duration
}

func (s *scanFixture) Scan(ctx context.Context, in *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	s.calls++
	if in.ReturnConsumedCapacity != types.ReturnConsumedCapacityTotal || in.Limit == nil || *in.Limit > 100 {
		return nil, errors.New("scan must request capacity and bounded pages")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := map[string]types.AttributeValue{"Platform": &types.AttributeValueMemberS{Value: "Reddit"}}
	if s.calls == 1 {
		s.firstCall = time.Now()
		return &dynamodb.ScanOutput{ScannedCount: 1, LastEvaluatedKey: key, ConsumedCapacity: &types.ConsumedCapacity{CapacityUnits: &s.consumed}}, nil
	}
	s.gap = time.Since(s.firstCall)
	if len(in.ExclusiveStartKey) == 0 {
		return nil, errors.New("missing continuation key")
	}
	item := func(url string) map[string]types.AttributeValue {
		return map[string]types.AttributeValue{"Platform": &types.AttributeValueMemberS{Value: "Reddit"}, "SortKey": &types.AttributeValueMemberS{Value: url}, "Title": &types.AttributeValueMemberS{Value: "legacy"}, "Timestamp": &types.AttributeValueMemberN{Value: "123"}}
	}
	lease := item("https://example.test/lease")
	lease["Platform"] = &types.AttributeValueMemberS{Value: "__grass_lambda_lock__"}
	budget := item("https://example.test/budget")
	budget["Platform"] = &types.AttributeValueMemberS{Value: "__grass_youtube_budget__"}
	return &dynamodb.ScanOutput{ScannedCount: 4, Items: []map[string]types.AttributeValue{item("LastSearchTime"), lease, budget, item("https://example.test")}}, nil
}

func TestArchiveScanPacing(t *testing.T) {
	client := &scanFixture{consumed: 0.1}
	reader := &DynamoDBArchiveReader{client: client, table: "test"}
	_, err := reader.Load(context.Background(), DefaultArchiveLimits())
	if err != nil {
		t.Fatal(err)
	}
	if client.gap < 45*time.Millisecond {
		t.Fatalf("scan pages were not paced: %v", client.gap)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if err := waitArchiveCapacity(ctx, 100, 2, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pacing ignored cancellation: %v", err)
	}
}
func TestDynamoArchivePaginationAndLimits(t *testing.T) {
	for _, limit := range []string{"none", "scanned", "bytes", "cancelled"} {
		t.Run(limit, func(t *testing.T) {
			client := &scanFixture{}
			reader := &DynamoDBArchiveReader{client: client, table: "test"}
			l := DefaultArchiveLimits()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch limit {
			case "scanned":
				l.Scanned = 2
			case "bytes":
				l.Bytes = 1
			case "cancelled":
				cancel()
			}
			got, err := reader.Load(ctx, l)
			if limit == "none" {
				if err != nil || len(got) != 1 || got[0].AuthorKey != "" || client.calls != 2 {
					t.Fatalf("%+v %v calls=%d", got, err, client.calls)
				}
			} else if err == nil || got != nil {
				t.Fatalf("partial data published: %+v %v", got, err)
			}
		})
	}
}
func TestReadOnlyLegacySQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy")
	if _, err := NewSQLiteArchiveReader(path); err == nil {
		t.Fatal("missing database opened")
	}
	if _, err := os.Stat(path + ".db"); !os.IsNotExist(err) {
		t.Fatal("reader created a database")
	}
	db, err := sql.Open("sqlite3", path+".db")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE search_results(Platform TEXT,Keyword TEXT,Title TEXT,URL TEXT PRIMARY KEY,Timestamp INTEGER); INSERT INTO search_results VALUES('Reddit','test','old','https://example.test/1',1),('Reddit','test','old','https://example.test/2',2)`)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reader, err := NewSQLiteArchiveReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	got, err := reader.Load(context.Background(), DefaultArchiveLimits())
	if err != nil || len(got) != 2 || got[0].Content != "" {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := db.Exec("SELECT Content FROM search_results"); err == nil {
		t.Fatal("reader migrated legacy schema")
	}
	l := DefaultArchiveLimits()
	l.Records = 1
	got, err = reader.Load(context.Background(), l)
	if err == nil || got != nil {
		t.Fatal("record limit did not reject incomplete snapshot")
	}
}
