package main

import (
	"log"

	"github.com/alecthomas/kingpin/v2"
	"github.com/jaxxstorm/grass/bot"
	"github.com/jaxxstorm/grass/search"
	"github.com/jaxxstorm/grass/storage"
	"github.com/joho/godotenv"
)

var (
	dbType    = kingpin.Flag("db", "Specify the database type to use: dynamodb or sqlite").Default("sqlite").Enum("dynamodb", "sqlite")
	keywords  = kingpin.Flag("keyword", "Specify keywords to search for").Strings()
	botTypes  = kingpin.Flag("bot", "Specify bot types to use: print, discord").Strings()
	searchers = kingpin.Flag("searchers", "Specify searchers to use: hackernews, reddit, bluesky").Strings()
	tableName = kingpin.Flag("table-name", "Specify the table name to use for SQLite storage").Envar("SOCIAL_SEARCH_TABLE_NAME").Default("grass").String()
)

func init() {
	// Load the .env file
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found or error reading it; make sure environment variables are set.")
	}
}

func main() {
	kingpin.Parse()

	// Initialize searchers
	var searchersList []search.Searcher
	for _, searcher := range *searchers {
		switch searcher {
		case "hackernews":
			searchersList = append(searchersList, search.NewHackerNewsSearcher())
		case "reddit":
			redditSearcher, err := search.NewRedditSearcher()
			if err != nil {
				log.Fatalf("Failed to initialize Reddit searcher: %v", err)
			}
			searchersList = append(searchersList, redditSearcher)
		case "bluesky":
			blueskySearcher, err := search.NewBlueskySearcher()
			if err != nil {
				log.Fatalf("Failed to initialize Bluesky searcher: %v", err)
			}
			searchersList = append(searchersList, blueskySearcher)
		default:
			log.Fatalf("Unknown searcher specified: %s", searcher)
		}
	}

	// Initialize the storage backend
	var storer storage.Storer
	var err error

	switch *dbType {
	case "dynamodb":
		storer, err = storage.NewDynamoDBStorer(*tableName)
		if err != nil {
			log.Fatalf("Failed to initialize DynamoDB storage: %v", err)
		}
	case "sqlite":
		storer, err = storage.NewSQLiteStorer(*tableName)
		if err != nil {
			log.Fatalf("Failed to initialize SQLite storage: %v", err)
		}
		defer func() {
			if err := storer.(*storage.SQLiteStorer).Close(); err != nil {
				log.Printf("Failed to close SQLite storage: %v", err)
			}
		}()
	default:
		log.Fatalf("Unknown database type: %s", *dbType)
	}

	// Initialize notifiers
	var notifiers []bot.Notifier
	for _, botType := range *botTypes {
		switch botType {
		case "print":
			notifiers = append(notifiers, bot.NewPrintNotifier())
		case "discord":
			notifiers = append(notifiers, bot.NewDiscordNotifier())
		default:
			log.Fatalf("Unknown bot type: %s", botType)
		}
	}

	// Run the bot
	b := bot.NewBot(searchersList, storer, notifiers)
	for _, keyword := range *keywords {
		log.Printf("Running search for keyword: %s", keyword)
		b.Run(keyword)
	}
}
