package webserver

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/jaxxstorm/grass/localindex"
	"os"
)

func dynamoSource(ctx context.Context, table string) (localindex.Source, aws.Config, error) {
	source := localindex.Source{Kind: "dynamodb", Table: table, Profile: os.Getenv("AWS_PROFILE")}
	// Load configuration locally; credentials remain lazy until synchronization.
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return source, cfg, err
	}
	source.Region = cfg.Region
	if source.Profile == "" {
		source.Profile = os.Getenv("AWS_DEFAULT_PROFILE")
	}
	source.Endpoint = aws.ToString(dynamodb.NewFromConfig(cfg).Options().BaseEndpoint)
	return source, cfg, nil
}
