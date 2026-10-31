package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/jaxxstorm/grass/search"
)

func TestSQLiteMetadataMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive")
	db, err := sql.Open("sqlite3", path+".db")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE search_results (Platform TEXT, Keyword TEXT, Title TEXT, URL TEXT PRIMARY KEY, Timestamp INTEGER); INSERT INTO search_results VALUES ('Reddit','test','old','https://example.test/old',1)`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	for i := 0; i < 2; i++ {
		s, err := NewSQLiteStorer(path)
		if err != nil {
			t.Fatal(err)
		}
		var title string
		if err := s.db.QueryRow("SELECT Title FROM search_results WHERE Timestamp=1").Scan(&title); err != nil || title != "old" {
			t.Fatalf("legacy record: %s %v", title, err)
		}
		r := search.SearchResult{Platform: "Reddit", URL: "https://example.test/new", Title: "new", Content: "body", AuthorID: "id", AuthorHandle: "alice", AuthorName: "Alice", Timestamp: 2}
		if err := s.Save(r); err != nil {
			t.Fatal(err)
		}
		var got search.SearchResult
		if err := s.db.QueryRow("SELECT Content, AuthorID, AuthorHandle, AuthorName FROM search_results WHERE Timestamp=2").Scan(&got.Content, &got.AuthorID, &got.AuthorHandle, &got.AuthorName); err != nil {
			t.Fatal(err)
		}
		if got.Content != r.Content || got.AuthorID != r.AuthorID || got.AuthorHandle != r.AuthorHandle || got.AuthorName != r.AuthorName {
			t.Fatalf("lost metadata: %+v", got)
		}
		s.Close()
	}
}

func TestDynamoMetadataWrite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		if r.Header.Get("X-Amz-Target") == "DynamoDB_20120810.GetItem" {
			w.Write([]byte(`{}`))
			return
		}
		var body struct {
			TransactItems []struct {
				Put struct{ Item map[string]map[string]string }
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.TransactItems) != 3 {
			t.Errorf("expected atomic result/journal transaction")
			w.WriteHeader(400)
			return
		}
		item := body.TransactItems[0].Put.Item
		for k, v := range map[string]string{"Content": "body", "AuthorID": "id", "AuthorHandle": "alice", "AuthorName": "Alice", "SortKey": "https://example.test"} {
			if item[k]["S"] != v {
				t.Errorf("%s lost: %v", k, item)
			}
		}
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client := dynamodb.NewFromConfig(aws.Config{Region: "us-west-2", Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
		return aws.Credentials{AccessKeyID: "test", SecretAccessKey: "test"}, nil
	})}, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String(server.URL) })
	s := &DynamoDBStorer{client: client, tableName: "grass"}
	if err := s.Save(search.SearchResult{Platform: "Reddit", URL: "https://example.test", Content: "body", AuthorID: "id", AuthorHandle: "alice", AuthorName: "Alice"}); err != nil {
		t.Fatal(err)
	}
}
