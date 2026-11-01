package archivesource

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"testing"
)

func TestReplayRejectsMismatchedWriteIdentity(t *testing.T) {
	for _, field := range []string{"JournalEpoch", "JournalSequence", "JournalWriteID"} {
		for _, value := range []types.AttributeValue{nil, &types.AttributeValueMemberS{Value: "wrong"}, &types.AttributeValueMemberN{Value: "2"}} {
			t.Run(field, func(t *testing.T) {
				f := &fakeDynamo{identityField: field, identityValue: value}
				d := &Dynamo{client: f, table: "table", budget: pacer{rate: 100}}
				if _, err := d.Changes(context.Background(), "epoch", 0, 1); !errors.Is(err, ErrJournal) {
					t.Fatalf("mismatched %s accepted: %v", field, err)
				}
			})
		}
	}
}
