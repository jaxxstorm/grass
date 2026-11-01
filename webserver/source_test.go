package webserver

import (
	"context"
	"github.com/jaxxstorm/grass/localindex"
	"os"
	"path/filepath"
	"testing"
)

func TestDynamoSourceUsesResolvedProfileRegion(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config")
	t.Setenv("AWS_CONFIG_FILE", configFile)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "missing"))
	t.Setenv("AWS_PROFILE", "archive-test")
	for _, name := range []string{"AWS_REGION", "AWS_DEFAULT_REGION", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_DYNAMODB"} {
		t.Setenv(name, "")
	}
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	var previous string
	for _, region := range []string{"us-east-1", "eu-west-1"} {
		if err := os.WriteFile(configFile, []byte("[profile archive-test]\nregion = "+region+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		source, cfg, err := dynamoSource(context.Background(), "grass")
		if err != nil {
			t.Fatal(err)
		}
		if source.Region != region || cfg.Region != region {
			t.Fatalf("source/config region mismatch: %q/%q want %q", source.Region, cfg.Region, region)
		}
		path, err := localindex.DefaultPath(source)
		if err != nil {
			t.Fatal(err)
		}
		if path == previous {
			t.Fatal("profile region change reused cache")
		}
		previous = path
	}
}
