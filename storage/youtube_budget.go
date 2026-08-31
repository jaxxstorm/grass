package storage

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"time"
	_ "time/tzdata"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const YouTubeDailyLimit = 99
const YouTubeSearchInterval = time.Hour

// YouTubeBudget reserves an attempt before calling the API. Failed calls count
// too. All callers must share a store to share the quota and keyword cooldowns.
type YouTubeBudget interface {
	ReserveYouTubeSearch(keyword string, now time.Time) (bool, error)
}

func youtubeBudgetKeys(keyword string, now time.Time) (string, string, error) {
	zone, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return "", "", err
	}
	return "day:" + now.In(zone).Format("2006-01-02"), fmt.Sprintf("keyword:%x", sha256.Sum256([]byte(keyword))), nil
}

func (d *DynamoDBStorer) ReserveYouTubeSearch(keyword string, now time.Time) (bool, error) {
	day, keywordKey, err := youtubeBudgetKeys(keyword, now)
	if err != nil {
		return false, err
	}
	key := func(value string) map[string]types.AttributeValue {
		return map[string]types.AttributeValue{
			"Platform": &types.AttributeValueMemberS{Value: "__grass_youtube_budget__"},
			"SortKey":  &types.AttributeValueMemberS{Value: value},
		}
	}
	number := func(value int64) types.AttributeValue {
		return &types.AttributeValueMemberN{Value: strconv.FormatInt(value, 10)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Both reservations succeed atomically; a cooldown skip cannot consume budget.
	_, err = d.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
		{Update: &types.Update{TableName: aws.String(d.tableName), Key: key(day),
			UpdateExpression:          aws.String("ADD #used :one"),
			ConditionExpression:       aws.String("attribute_not_exists(#used) OR #used < :limit"),
			ExpressionAttributeNames:  map[string]string{"#used": "Used"},
			ExpressionAttributeValues: map[string]types.AttributeValue{":one": number(1), ":limit": number(YouTubeDailyLimit)},
		}},
		{Update: &types.Update{TableName: aws.String(d.tableName), Key: key(keywordKey),
			UpdateExpression:          aws.String("SET #next = :next"),
			ConditionExpression:       aws.String("attribute_not_exists(#next) OR #next <= :now"),
			ExpressionAttributeNames:  map[string]string{"#next": "NextAllowed"},
			ExpressionAttributeValues: map[string]types.AttributeValue{":now": number(now.Unix()), ":next": number(now.Add(YouTubeSearchInterval).Unix())},
		}},
	}})
	var cancelled *types.TransactionCanceledException
	if errors.As(err, &cancelled) {
		for _, reason := range cancelled.CancellationReasons {
			if aws.ToString(reason.Code) == "ConditionalCheckFailed" {
				return false, nil
			}
		}
	}
	return err == nil, err
}

func (s *SQLiteStorer) ReserveYouTubeSearch(keyword string, now time.Time) (bool, error) {
	day, keywordKey, err := youtubeBudgetKeys(keyword, now)
	if err != nil {
		return false, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`INSERT INTO youtube_budget (Key, Value) VALUES (?, 1)
		ON CONFLICT(Key) DO UPDATE SET Value = Value + 1 WHERE Value < ?`, day, YouTubeDailyLimit)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	result, err = tx.Exec(`INSERT INTO youtube_budget (Key, Value) VALUES (?, ?)
		ON CONFLICT(Key) DO UPDATE SET Value = excluded.Value WHERE Value <= ?`, keywordKey, now.Add(YouTubeSearchInterval).Unix(), now.Unix())
	if err != nil {
		return false, err
	}
	n, err = result.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	err = tx.Commit()
	return err == nil, err
}
