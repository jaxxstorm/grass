package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kingpin/v2"
	"github.com/charmbracelet/log"
	"github.com/jaxxstorm/grass/app"
	"github.com/joho/godotenv"
)

var (
	Version     = "dev"
	dbType      = kingpin.Flag("db", "Specify the database type to use: dynamodb or sqlite").Default("sqlite").Enum("dynamodb", "sqlite")
	keywords    = kingpin.Flag("keyword", "Specify keywords to search for").Strings()
	botTypes    = kingpin.Flag("bot", "Specify bot types to use: print, discord").Strings()
	searchers   = kingpin.Flag("searchers", "Specify searchers to use: hackernews, reddit, bluesky, fediverse, youtube, x").Strings()
	tableName   = kingpin.Flag("table-name", "Specify the table name to use for SQLite storage").Envar("SOCIAL_SEARCH_TABLE_NAME").Default("grass").String()
	showVersion = kingpin.Flag("version", "Show the version and exit").Bool()
)

func init() {
	// Load the .env file
	err := godotenv.Load()
	if err != nil {
		log.Debug("No .env file found or error reading it; make sure environment variables are set.")
	}
}

func main() {
	kingpin.Parse()

	if *showVersion {
		fmt.Println("Version:", Version)
		os.Exit(0)
	}

	if err := app.Run(app.Config{
		Database:  *dbType,
		TableName: *tableName,
		Keywords:  *keywords,
		Bots:      *botTypes,
		Searchers: *searchers,
	}); err != nil {
		log.Fatalf("Failed to run Grass: %v", err)
	}
}
