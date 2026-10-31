package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/charmbracelet/log"
	"github.com/jaxxstorm/grass/search"
)

// ArchiveReader is intentionally independent from the collector's write API.
type ArchiveReader interface {
	Load(context.Context, ArchiveLimits) ([]ArchiveRecord, error)
	Close() error
}

type ArchiveRecord struct {
	search.SearchResult
	ID        string
	AuthorKey string
}

type ArchiveLimits struct {
	Records, Scanned, Bytes int
	Timeout                 time.Duration
	ReadCapacity            float64
}

func DefaultArchiveLimits() ArchiveLimits {
	return ArchiveLimits{Records: 50000, Scanned: 100000, Bytes: 64 << 20, Timeout: 10 * time.Minute, ReadCapacity: 2}
}
func (l ArchiveLimits) Validate() error {
	if l.Records <= 0 || l.Scanned <= 0 || l.Bytes <= 0 || l.Timeout <= 0 || l.ReadCapacity <= 0 || math.IsNaN(l.ReadCapacity) || math.IsInf(l.ReadCapacity, 0) {
		return errors.New("archive limits must be positive")
	}
	return nil
}

type ArchiveLimitError struct{ Limit string }

func (e *ArchiveLimitError) Error() string {
	return "archive exceeds " + e.Limit + " limit; increase the corresponding --archive limit or use a smaller archive"
}

type archiveBuffer struct {
	limits         ArchiveLimits
	scanned, bytes int
	records        []ArchiveRecord
	seen           map[string]bool
}

func (b *archiveBuffer) examine(n int) error {
	b.scanned += n
	if b.scanned > b.limits.Scanned {
		return &ArchiveLimitError{"scanned items"}
	}
	return nil
}
func (b *archiveBuffer) add(r search.SearchResult) error {
	u, err := url.Parse(r.URL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || r.Platform == "" || strings.HasPrefix(r.Platform, "__grass_") {
		return nil
	}
	key := r.Platform + "\x00" + r.URL
	if b.seen[key] {
		return nil
	}
	size := len(r.Platform) + len(r.Keyword) + len(r.Title) + len(r.URL) + len(r.Content) + len(r.AuthorID) + len(r.AuthorHandle) + len(r.AuthorName) + 8
	if size > b.limits.Bytes-b.bytes {
		return &ArchiveLimitError{"bytes"}
	}
	if len(b.records) >= b.limits.Records {
		return &ArchiveLimitError{"records"}
	}
	b.bytes += size
	b.seen[key] = true
	r.Title = ArchivePlainText(r.Title)
	r.Content = ArchivePlainText(r.Content)
	r.AuthorName = ArchivePlainText(r.AuthorName)
	hash := sha256.Sum256([]byte(key))
	b.records = append(b.records, ArchiveRecord{SearchResult: r, ID: hex.EncodeToString(hash[:]), AuthorKey: r.AuthorKey()})
	return nil
}
func newArchiveBuffer(l ArchiveLimits) *archiveBuffer {
	return &archiveBuffer{limits: l, records: []ArchiveRecord{}, seen: map[string]bool{}}
}

type SQLiteArchiveReader struct{ db *sql.DB }

func NewSQLiteArchiveReader(path string) (*SQLiteArchiveReader, error) {
	abs, err := filepath.Abs(path + ".db")
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: abs, RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite3", u.String())
	if err != nil {
		return nil, err
	}
	if err = db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open read-only archive: %w", err)
	}
	return &SQLiteArchiveReader{db}, nil
}
func (s *SQLiteArchiveReader) Close() error { return s.db.Close() }
func (s *SQLiteArchiveReader) Load(ctx context.Context, l ArchiveLimits) ([]ArchiveRecord, error) {
	if err := l.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, l.Timeout)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "PRAGMA table_info(search_results)")
	if err != nil {
		return nil, err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid, nn, pk int
		var name, typ string
		var def sql.NullString
		if err = rows.Scan(&cid, &name, &typ, &nn, &def, &pk); err != nil {
			rows.Close()
			return nil, err
		}
		columns[strings.ToLower(name)] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if !columns["url"] {
		return nil, errors.New("archive search_results table is missing")
	}
	selects := []string{"COALESCE(Platform,'')", "COALESCE(Keyword,'')", "COALESCE(Title,'')", "URL", "COALESCE(Timestamp,0)"}
	for _, name := range []string{"Content", "AuthorID", "AuthorHandle", "AuthorName"} {
		if columns[strings.ToLower(name)] {
			selects = append(selects, "COALESCE("+name+",'')")
		} else {
			selects = append(selects, "''")
		}
	}
	rows, err = tx.QueryContext(ctx, "SELECT "+strings.Join(selects, ",")+" FROM search_results")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	b := newArchiveBuffer(l)
	for rows.Next() {
		if err = b.examine(1); err != nil {
			return nil, err
		}
		var r search.SearchResult
		if err = rows.Scan(&r.Platform, &r.Keyword, &r.Title, &r.URL, &r.Timestamp, &r.Content, &r.AuthorID, &r.AuthorHandle, &r.AuthorName); err != nil {
			return nil, err
		}
		if err = b.add(r); err != nil {
			return nil, err
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return b.records, nil
}

type archiveScanner interface {
	Scan(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
}
type DynamoDBArchiveReader struct {
	client archiveScanner
	table  string
}

func NewDynamoDBArchiveReader(ctx context.Context, table string) (*DynamoDBArchiveReader, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	if cfg.Region == "" {
		return nil, fmt.Errorf("AWS region is not configured; set AWS_REGION or configure a region in the active AWS profile")
	}
	log.Info("Opening DynamoDB archive", "table", table, "region", cfg.Region)
	return &DynamoDBArchiveReader{dynamodb.NewFromConfig(cfg), table}, nil
}
func (d *DynamoDBArchiveReader) Close() error { return nil }
func (d *DynamoDBArchiveReader) Load(ctx context.Context, l ArchiveLimits) ([]ArchiveRecord, error) {
	if err := l.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, l.Timeout)
	defer cancel()
	b := newArchiveBuffer(l)
	input := &dynamodb.ScanInput{TableName: aws.String(d.table), ProjectionExpression: aws.String("#p,#s,#k,#t,#ts,#c,#ai,#ah,#an"), ExpressionAttributeNames: map[string]string{"#p": "Platform", "#s": "SortKey", "#k": "Keyword", "#t": "Title", "#ts": "Timestamp", "#c": "Content", "#ai": "AuthorID", "#ah": "AuthorHandle", "#an": "AuthorName"}}
	input.ReturnConsumedCapacity = types.ReturnConsumedCapacityTotal
	lastProgress := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		input.Limit = aws.Int32(int32(min(100, l.Scanned-b.scanned+1)))
		started := time.Now()
		page, err := d.client.Scan(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("scan DynamoDB table %q after %d examined items: %w", d.table, b.scanned, err)
		}
		if err = b.examine(max(int(page.ScannedCount), len(page.Items))); err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			str := func(k string) string {
				v, _ := item[k].(*types.AttributeValueMemberS)
				if v == nil {
					return ""
				}
				return v.Value
			}
			n, ok := item["Timestamp"].(*types.AttributeValueMemberN)
			if !ok {
				continue
			}
			timestamp, err := strconv.ParseInt(n.Value, 10, 64)
			if err != nil {
				continue
			}
			r := search.SearchResult{Platform: str("Platform"), URL: str("SortKey"), Keyword: str("Keyword"), Title: str("Title"), Timestamp: timestamp, Content: str("Content"), AuthorID: str("AuthorID"), AuthorHandle: str("AuthorHandle"), AuthorName: str("AuthorName")}
			if err = b.add(r); err != nil {
				return nil, err
			}
		}
		if len(page.LastEvaluatedKey) == 0 {
			break
		}
		input.ExclusiveStartKey = page.LastEvaluatedKey
		if time.Since(lastProgress) >= 10*time.Second {
			log.Info("Loading DynamoDB archive", "table", d.table, "examined", b.scanned, "records", len(b.records), "read_capacity_per_second", l.ReadCapacity)
			lastProgress = time.Now()
		}
		// Projection does not reduce read capacity: pace using the capacity the
		// table actually consumed, accounting for time spent in the request.
		if page.ConsumedCapacity != nil {
			if err := waitArchiveCapacity(ctx, aws.ToFloat64(page.ConsumedCapacity.CapacityUnits), l.ReadCapacity, time.Since(started)); err != nil {
				return nil, err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return b.records, nil
}

func waitArchiveCapacity(ctx context.Context, consumed, perSecond float64, elapsed time.Duration) error {
	wait := time.Duration(consumed/perSecond*float64(time.Second)) - elapsed
	if wait <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
