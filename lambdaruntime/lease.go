package lambdaruntime

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type leaseClient interface {
	PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

// acquireLease returns a release function, or nil when another invocation owns the lease.
func acquireLease(ctx context.Context, table string) (func() error, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return acquire(ctx, dynamodb.NewFromConfig(cfg), table, time.Now())
}

func acquire(ctx context.Context, client leaseClient, table string, now time.Time) (func() error, error) {
	// Lambda enforces a maximum 15-minute execution time. The grace period
	// prevents takeover before the previous runtime has been terminated.
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = now.Add(15 * time.Minute)
	}
	owner := rand.Text()
	key := map[string]types.AttributeValue{
		"Platform": &types.AttributeValueMemberS{Value: "__grass_lambda__"},
		"SortKey":  &types.AttributeValueMemberS{Value: "RunLease"},
	}
	item := map[string]types.AttributeValue{
		"Platform": key["Platform"], "SortKey": key["SortKey"],
		"Owner":     &types.AttributeValueMemberS{Value: owner},
		"ExpiresAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(deadline.Add(time.Minute).Unix(), 10)},
	}
	_, err := client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(table), Item: item,
		ConditionExpression:       aws.String("attribute_not_exists(#pk) OR #expires < :now"),
		ExpressionAttributeNames:  map[string]string{"#pk": "Platform", "#expires": "ExpiresAt"},
		ExpressionAttributeValues: map[string]types.AttributeValue{":now": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Unix(), 10)}},
	})
	var conflict *types.ConditionalCheckFailedException
	if errors.As(err, &conflict) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("acquire run lease: %w", err)
	}
	return func() error {
		// Cleanup remains possible even when the invocation context is cancelled.
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := client.DeleteItem(cleanup, &dynamodb.DeleteItemInput{
			TableName: aws.String(table), Key: key,
			ConditionExpression:       aws.String("#owner = :owner"),
			ExpressionAttributeNames:  map[string]string{"#owner": "Owner"},
			ExpressionAttributeValues: map[string]types.AttributeValue{":owner": &types.AttributeValueMemberS{Value: owner}},
		})
		if errors.As(err, &conflict) {
			return nil // This invocation no longer owns the lease.
		}
		return err
	}, nil
}
