package storage

import (
	"context"
	"encoding/json"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDynamoYouTubeContinuationBudget(t *testing.T) {
	for _, denied := range []bool{false, true} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			var in struct {
				TransactItems []struct {
					Update struct {
						ConditionExpression       string
						Key                       map[string]map[string]string
						ExpressionAttributeValues map[string]map[string]string
					}
				}
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			if len(in.TransactItems) != 1 {
				t.Error("continuation must reserve only daily budget")
				w.WriteHeader(400)
				return
			}
			daily := in.TransactItems[0].Update
			if daily.ConditionExpression != "attribute_not_exists(#used) OR #used < :limit" || daily.ExpressionAttributeValues[":limit"]["N"] != "99" {
				t.Error("daily cap missing")
			}
			if daily.Key["Platform"]["S"] != "__grass_youtube_budget__" {
				t.Error("wrong budget partition")
			}
			w.Header().Set("Content-Type", "application/x-amz-json-1.0")
			if denied {
				w.WriteHeader(400)
				w.Write([]byte(`{"__type":"TransactionCanceledException","CancellationReasons":[{"Code":"ConditionalCheckFailed"}]}`))
			} else {
				w.Write([]byte(`{}`))
			}
		}))
		client := dynamodb.NewFromConfig(aws.Config{Region: "us-east-1", Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "test", SecretAccessKey: "test"}, nil
		})}, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String(server.URL) })
		s := &DynamoDBStorer{client: client, tableName: "grass"}
		allowed, err := s.ReserveYouTubePage(time.Now())
		server.Close()
		if err != nil || allowed == denied || calls != 1 {
			t.Fatalf("allowed=%v err=%v calls=%d", allowed, err, calls)
		}
	}
}
