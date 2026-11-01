package lambdaruntime

import (
	"context"
	"errors"
	"github.com/aws/aws-lambda-go/events"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/jaxxstorm/grass/app"
)

type fakeLeaseClient struct {
	put *dynamodb.PutItemInput
	del *dynamodb.DeleteItemInput
	err error
}

func (f *fakeLeaseClient) PutItem(_ context.Context, in *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.put = in
	return nil, f.err
}
func (f *fakeLeaseClient) DeleteItem(_ context.Context, in *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	f.del = in
	return nil, f.err
}

func TestLeaseConditionalOwnershipAndDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(2000000000, 0))
	defer cancel()
	f := &fakeLeaseClient{}
	release, err := acquire(ctx, f, "grass", time.Unix(1999999900, 0))
	if err != nil {
		t.Fatal(err)
	}
	if *f.put.ConditionExpression != "attribute_not_exists(#pk) OR #expires < :now" {
		t.Fatal("acquisition must atomically check expiration")
	}
	if got := f.put.Item["ExpiresAt"].(*types.AttributeValueMemberN).Value; got != "2000000060" {
		t.Fatalf("expiry = %s", got)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if *f.del.ConditionExpression != "#owner = :owner" || f.del.ExpressionAttributeValues[":owner"].(*types.AttributeValueMemberS).Value != f.put.Item["Owner"].(*types.AttributeValueMemberS).Value {
		t.Fatal("release must only delete the acquiring owner's lease")
	}
	f.err = &types.ConditionalCheckFailedException{}
	if err := release(); err != nil {
		t.Fatalf("lost ownership should be harmless: %v", err)
	}
}

func TestLeaseConflictAndFailure(t *testing.T) {
	for _, err := range []error{&types.ConditionalCheckFailedException{}, errors.New("unavailable")} {
		f := &fakeLeaseClient{err: err}
		release, got := acquire(context.Background(), f, "grass", time.Now())
		if release != nil {
			t.Fatal("failed acquisition returned release")
		}
		var conflict *types.ConditionalCheckFailedException
		if errors.As(err, &conflict) {
			if got != nil {
				t.Fatal(got)
			}
		} else if !errors.Is(got, err) {
			t.Fatalf("lost acquisition error: %v", got)
		}
	}
}

func TestHandlerLeaseLifecycle(t *testing.T) {
	for _, mode := range []string{"busy", "failure", "success", "run failure"} {
		t.Run(mode, func(t *testing.T) {
			called, released := false, false
			values := map[string]string{"SOCIAL_SEARCH_TABLE_NAME": "grass", "GRASS_KEYWORDS": "grass", "GRASS_SEARCHERS": "hackernews", "GRASS_BOTS": "print"}
			h := Handler{
				getenv: func(k string) string { return values[k] },
				acquire: func(context.Context, string) (func() error, error) {
					if mode == "busy" {
						return nil, nil
					}
					if mode == "failure" {
						return nil, errors.New("unavailable")
					}
					return func() error { released = true; return nil }, nil
				},
				run: func(app.Config) error {
					called = true
					if mode == "run failure" {
						return errors.New("run failed")
					}
					return nil
				},
			}
			err := h.Handle(context.Background(), events.EventBridgeEvent{})
			wantRun := mode == "success" || mode == "run failure"
			if called != wantRun || released != wantRun {
				t.Fatalf("called=%v released=%v", called, released)
			}
			if (err != nil) != (mode == "failure" || mode == "run failure") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
