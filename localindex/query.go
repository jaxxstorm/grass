package localindex

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jaxxstorm/grass/archive"
	"github.com/jaxxstorm/grass/storage"
)

func searchWhere(q archive.Query) (string, []any, error) {
	if q.Page < 1 || q.PageSize < 1 || q.PageSize > 100 || q.Page-1 > math.MaxInt/q.PageSize || utf8.RuneCountInString(q.Text) > 256 {
		return "", nil, fmt.Errorf("invalid pagination or search length")
	}
	parts := []string{"1=1"}
	args := []any{}
	if q.Text != "" {
		tokens := strings.FieldsFunc(q.Text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
		if len(tokens) == 0 {
			parts = append(parts, "0=1")
		} else {
			for j, t := range tokens {
				tokens[j] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
			}
			parts = append(parts, "r.rowid IN (SELECT rowid FROM records_fts WHERE records_fts MATCH ?)")
			args = append(args, strings.Join(tokens, " AND "))
		}
	}
	for _, f := range []struct {
		column string
		values []string
	}{{"platform", q.Platforms}, {"keyword", q.Keywords}} {
		if len(f.values) == 0 {
			continue
		}
		marks := make([]string, len(f.values))
		for j, v := range f.values {
			marks[j] = "?"
			args = append(args, v)
		}
		parts = append(parts, "r."+f.column+" IN ("+strings.Join(marks, ",")+")")
	}
	if q.Author != "" {
		parts = append(parts, "r.author_key=?")
		args = append(args, q.Author)
	}
	if q.Term != "" {
		parts = append(parts, "EXISTS(SELECT 1 FROM record_terms t WHERE t.record_id=r.id AND t.term=?)")
		args = append(args, q.Term)
	}
	if !q.From.IsZero() {
		parts = append(parts, "r.timestamp>=?")
		args = append(args, float64(q.From.Unix())+float64(q.From.Nanosecond())/1e9)
	}
	if !q.To.IsZero() {
		parts = append(parts, "r.timestamp<?")
		args = append(args, float64(q.To.Unix())+float64(q.To.Nanosecond())/1e9)
	}
	return strings.Join(parts, " AND "), args, nil
}

func (i *Index) Query(ctx context.Context, q archive.Query) (archive.Response, error) {
	out := archive.Response{Results: []storage.ArchiveRecord{}, Keywords: []archive.Count{}, Platforms: []archive.Count{}, Authors: []archive.Count{}, Terms: []archive.Count{}, Activity: []archive.Count{}, Page: q.Page, PageSize: q.PageSize}
	where, args, err := searchWhere(q)
	if err != nil {
		return out, err
	}
	tx, err := i.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	s, _, err := decodeState(ctx, tx)
	if err != nil {
		return out, err
	}
	if s.Generation == "" {
		return out, ErrNotReady
	}
	if q.Snapshot != "" && q.Snapshot != s.Generation {
		return out, ErrSnapshot
	}
	out.Snapshot = s.Generation
	out.LoadedAt = s.LoadedAt
	out.ArchiveTotal = s.Records
	err = tx.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(author_key!=''),0),coalesce(sum(author_key=''),0),coalesce(sum(content!=''),0),coalesce(sum(content=''),0) FROM records r WHERE `+where, args...).Scan(&out.Total, &out.Coverage.KnownAuthors, &out.Coverage.UnknownAuthors, &out.Coverage.WithContent, &out.Coverage.WithoutContent)
	if err != nil {
		return out, err
	}
	pageArgs := append(append([]any{}, args...), q.PageSize, (q.Page-1)*q.PageSize)
	rows, err := tx.QueryContext(ctx, `SELECT id,platform,url,keyword,title,timestamp,content,author_id,author_handle,author_name,author_key FROM records r WHERE `+where+` ORDER BY timestamp DESC,platform,url LIMIT ? OFFSET ?`, pageArgs...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var r storage.ArchiveRecord
		if err = rows.Scan(&r.ID, &r.Platform, &r.URL, &r.Keyword, &r.Title, &r.Timestamp, &r.Content, &r.AuthorID, &r.AuthorHandle, &r.AuthorName, &r.AuthorKey); err != nil {
			rows.Close()
			return out, err
		}
		out.Results = append(out.Results, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, group := range []struct {
		column string
		out    *[]archive.Count
	}{{"keyword", &out.Keywords}, {"platform", &out.Platforms}} {
		rows, err = tx.QueryContext(ctx, "SELECT "+group.column+",count(*) FROM records r WHERE "+where+" GROUP BY "+group.column+" ORDER BY count(*) DESC,"+group.column, args...)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var c archive.Count
			if err = rows.Scan(&c.Key, &c.Count); err != nil {
				rows.Close()
				return out, err
			}
			c.Label = c.Key
			*group.out = append(*group.out, c)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
	}
	rows, err = tx.QueryContext(ctx, `SELECT author_key,min(coalesce(nullif(author_handle,''),nullif(author_name,''),author_id)),platform,count(*) FROM records r WHERE `+where+` AND author_key!='' GROUP BY author_key,platform ORDER BY count(*) DESC,platform,author_key LIMIT 20`, args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var c archive.Count
		if err = rows.Scan(&c.Key, &c.Label, &c.Platform, &c.Count); err != nil {
			rows.Close()
			return out, err
		}
		out.Authors = append(out.Authors, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT t.term,count(*) FROM record_terms t JOIN records r ON r.id=t.record_id WHERE `+where+` GROUP BY t.term ORDER BY count(*) DESC,t.term LIMIT 50`, args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var c archive.Count
		if err = rows.Scan(&c.Key, &c.Count); err != nil {
			rows.Close()
			return out, err
		}
		c.Label = c.Key
		out.Terms = append(out.Terms, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	// Dates beyond this domain or a century of daily buckets are not valid
	// chart input. Fail explicitly rather than building unbounded output.
	var first, last sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT min(timestamp),max(timestamp) FROM records r WHERE `+where, args...).Scan(&first, &last)
	if err != nil {
		return out, err
	}
	if first.Valid {
		if first.Int64 < -62135596800 || last.Int64 > 253402300799 || last.Int64-first.Int64 > 36600*86400 {
			return out, ErrDateRange
		}
		rows, err = tx.QueryContext(ctx, `SELECT date(timestamp,'unixepoch'),count(*) FROM records r WHERE `+where+` GROUP BY date(timestamp,'unixepoch')`, args...)
		if err != nil {
			return out, err
		}
		days := map[string]int{}
		for rows.Next() {
			var day string
			var n int
			if err = rows.Scan(&day, &n); err != nil {
				rows.Close()
				return out, err
			}
			days[day] = n
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
		start := time.Unix(first.Int64, 0).UTC()
		start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
		end := time.Unix(last.Int64, 0).UTC()
		for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
			key := day.Format("2006-01-02")
			out.Activity = append(out.Activity, archive.Count{Key: key, Label: key, Count: days[key]})
		}
	}
	return out, tx.Commit()
}
