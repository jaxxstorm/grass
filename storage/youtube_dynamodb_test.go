package storage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func TestDynamoDBYouTubeBudgetAtomicReservation(t *testing.T) {
	for _, mode := range []string{"allowed", "cooldown", "daily limit", "denied"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("X-Amz-Target") != "DynamoDB_20120810.TransactWriteItems" {
					t.Error("reservation must use one transaction")
				}
				var body struct {
					TransactItems []struct {
						Update struct {
							ConditionExpression       string
							Key                       map[string]map[string]string
							ExpressionAttributeValues map[string]map[string]string
						}
					}
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if len(body.TransactItems) != 2 {
					t.Fatal("expected daily and keyword reservation together")
				}
				daily, keyword := body.TransactItems[0].Update, body.TransactItems[1].Update
				if daily.ExpressionAttributeValues[":limit"]["N"] != "99" || daily.ConditionExpression != "attribute_not_exists(#used) OR #used < :limit" {
					t.Error("missing daily bound")
				}
				if keyword.ConditionExpression != "attribute_not_exists(#next) OR #next <= :now" {
					t.Error("missing cooldown condition")
				}
				w.Header().Set("Content-Type", "application/x-amz-json-1.0")
				switch mode {
				case "allowed":
					w.Write([]byte(`{}`))
				case "cooldown":
					w.WriteHeader(400)
					w.Write([]byte(`{"__type":"TransactionCanceledException","CancellationReasons":[{"Code":"None"},{"Code":"ConditionalCheckFailed"}]}`))
				case "daily limit":
					w.WriteHeader(400)
					w.Write([]byte(`{"__type":"TransactionCanceledException","CancellationReasons":[{"Code":"ConditionalCheckFailed"},{"Code":"None"}]}`))
				case "denied":
					w.WriteHeader(400)
					w.Write([]byte(`{"__type":"AccessDeniedException","message":"denied"}`))
				}
			}))
			defer server.Close()
			client := dynamodb.NewFromConfig(aws.Config{Region: "us-west-2", Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
				return aws.Credentials{AccessKeyID: "test", SecretAccessKey: "test"}, nil
			})}, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String(server.URL) })
			s := &DynamoDBStorer{client: client, tableName: "grass"}
			allowed, err := s.ReserveYouTubeSearch("tailscale", time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC))
			if allowed != (mode == "allowed") || (err != nil) != (mode == "denied") || calls != 1 {
				t.Fatalf("allowed=%v error=%v calls=%d", allowed, err, calls)
			}
		})
	}
}
