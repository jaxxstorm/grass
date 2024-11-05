package storage

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/jaxxstorm/grass/search"
)

type DynamoDBStorer struct {
	client    *dynamodb.Client
	tableName string
}

func NewDynamoDBStorer(dbName string) (*DynamoDBStorer, error) {
	ctx := context.TODO()

	// Load AWS config with detailed logging
	cfg, err := config.LoadDefaultConfig(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := dynamodb.NewFromConfig(cfg)

	return &DynamoDBStorer{
		client:    client,
		tableName: dbName,
	}, nil
}

// Exists checks if a specific item (platform + URL) already exists in DynamoDB.
func (d *DynamoDBStorer) Exists(platform, url string) (bool, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"Platform": &types.AttributeValueMemberS{Value: platform},
			"SortKey":  &types.AttributeValueMemberS{Value: url},
		},
	}

	result, err := d.client.GetItem(context.TODO(), input)
	if err != nil {
		return false, fmt.Errorf("failed to get item from DynamoDB: %w", err)
	}

	return result.Item != nil, nil
}

// Save stores a new search result in DynamoDB.
func (d *DynamoDBStorer) Save(result search.SearchResult) error {
	item := map[string]types.AttributeValue{
		"Platform":  &types.AttributeValueMemberS{Value: result.Platform},
		"SortKey":   &types.AttributeValueMemberS{Value: result.URL},
		"Keyword":   &types.AttributeValueMemberS{Value: result.Keyword},
		"Title":     &types.AttributeValueMemberS{Value: result.Title},
		"Timestamp": &types.AttributeValueMemberN{Value: strconv.FormatInt(result.Timestamp, 10)},
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(d.tableName),
		Item:      item,
	}

	_, err := d.client.PutItem(context.TODO(), input)
	if err != nil {
		return fmt.Errorf("failed to put item into DynamoDB: %w", err)
	}
	return nil
}

// GetLastSearchTime retrieves the last search time for a given platform from DynamoDB.
func (d *DynamoDBStorer) GetLastSearchTime(platform string) (int64, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"Platform": &types.AttributeValueMemberS{Value: platform},
			"SortKey":  &types.AttributeValueMemberS{Value: "LastSearchTime"},
		},
	}

	result, err := d.client.GetItem(context.TODO(), input)
	if err != nil {
		return 0, fmt.Errorf("failed to get item from DynamoDB: %w", err)
	}

	if result.Item == nil {
		return 0, nil
	}

	lastSearchTimeStr, ok := result.Item["Timestamp"].(*types.AttributeValueMemberN)
	if !ok {
		return 0, fmt.Errorf("failed to parse Timestamp attribute from DynamoDB result")
	}

	lastSearchTime, err := strconv.ParseInt(lastSearchTimeStr.Value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse LastSearchTime: %w", err)
	}

	return lastSearchTime, nil
}

// SetLastSearchTime updates the last search time for a given platform in DynamoDB.
func (d *DynamoDBStorer) SetLastSearchTime(platform string, epochTime int64) error {
	item := map[string]types.AttributeValue{
		"Platform":  &types.AttributeValueMemberS{Value: platform},
		"SortKey":   &types.AttributeValueMemberS{Value: "LastSearchTime"},
		"Timestamp": &types.AttributeValueMemberN{Value: strconv.FormatInt(epochTime, 10)},
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(d.tableName),
		Item:      item,
	}

	_, err := d.client.PutItem(context.TODO(), input)
	if err != nil {
		return fmt.Errorf("failed to put item into DynamoDB: %w", err)
	}
	return nil
}
