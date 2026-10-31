// Package archive provides snapshot-based search over collected results.
package archive

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jaxxstorm/grass/storage"
)

type Query struct {
	Text                   string
	Platforms, Keywords    []string
	Author, Term, Snapshot string
	From, To               time.Time
	Page, PageSize         int
}

func ParseQuery(v url.Values) (Query, error) {
	q := Query{Text: v.Get("q"), Platforms: v["platform"], Keywords: v["keyword"], Author: v.Get("author"), Term: v.Get("term"), Snapshot: v.Get("snapshot"), Page: 1, PageSize: 25}
	if utf8.RuneCountInString(q.Text) > 256 {
		return q, fmt.Errorf("q must be at most 256 characters")
	}
	for _, f := range []struct {
		name string
		p    *int
	}{{"page", &q.Page}, {"page_size", &q.PageSize}} {
		if s := v.Get(f.name); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 {
				return q, fmt.Errorf("%s must be a positive integer", f.name)
			}
			*f.p = n
		}
	}
	if q.PageSize > 100 {
		return q, fmt.Errorf("page_size must be at most 100")
	}
	for _, f := range []struct {
		name string
		p    *time.Time
	}{{"from", &q.From}, {"to", &q.To}} {
		if s := v.Get(f.name); s != "" {
			t, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return q, fmt.Errorf("%s must be RFC3339", f.name)
			}
			*f.p = t
		}
	}
	if !q.From.IsZero() && !q.To.IsZero() && !q.From.Before(q.To) {
		return q, fmt.Errorf("from must precede to")
	}
	if q.Term != "" {
		tokens := Tokens(q.Term)
		if len(tokens) != 1 {
			return q, fmt.Errorf("term must be one cloud token")
		}
		for term := range tokens {
			q.Term = term
		}
	}
	return q, nil
}

// StopwordsVersion changes when the cloud vocabulary rules change.
const StopwordsVersion = "en-v1"

var stopwords = func() map[string]bool {
	m := map[string]bool{}
	for _, s := range strings.Fields("the and for that this with from have has had are was were been being not but you your yours they their them its it's our ours out into about also can could would should will just than then when where what which who how all any some more most only very there here these those such through over under post comment https http www com org") {
		m[s] = true
	}
	return m
}()
var cloudURLs = regexp.MustCompile(`(?i)(?:https?://|www\.)\S+`)

func Tokens(s string) map[string]bool {
	s = cloudURLs.ReplaceAllString(storage.ArchivePlainText(s), " ")
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	tokens := map[string]bool{}
	for _, word := range words {
		n := utf8.RuneCountInString(word)
		if n < 3 || n > 40 || stopwords[word] {
			continue
		}
		hasLetter := false
		for _, r := range word {
			if unicode.IsLetter(r) {
				hasLetter = true
				break
			}
		}
		if hasLetter {
			tokens[word] = true
		}
	}
	return tokens
}

type Count struct {
	Key      string
	Label    string
	Platform string `json:",omitempty"`
	Count    int
}
type Coverage struct{ KnownAuthors, UnknownAuthors, WithContent, WithoutContent int }
type Response struct {
	Snapshot                                      string
	LoadedAt                                      time.Time
	Results                                       []storage.ArchiveRecord
	Total, ArchiveTotal, Page, PageSize           int
	Keywords, Platforms, Authors, Terms, Activity []Count
	Coverage                                      Coverage
}

func counts(m map[string]int, limit int) []Count {
	r := []Count{}
	for k, v := range m {
		r = append(r, Count{Key: k, Label: k, Count: v})
	}
	sort.Slice(r, func(i, j int) bool {
		if r[i].Count != r[j].Count {
			return r[i].Count > r[j].Count
		}
		return r[i].Key < r[j].Key
	})
	if limit > 0 && len(r) > limit {
		r = r[:limit]
	}
	return r
}
func includes(values []string, s string) bool {
	if len(values) == 0 {
		return true
	}
	for _, v := range values {
		if v == s {
			return true
		}
	}
	return false
}
func Evaluate(records []storage.ArchiveRecord, q Query) Response {
	out := Response{Results: []storage.ArchiveRecord{}, Page: q.Page, PageSize: q.PageSize, ArchiveTotal: len(records), Activity: []Count{}}
	keywords, platforms, authors, terms, days := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	authorLabels := map[string]Count{}
	var first, last time.Time
	for _, r := range records {
		if !includes(q.Platforms, r.Platform) || !includes(q.Keywords, r.Keyword) || (q.Author != "" && r.AuthorKey != q.Author) {
			continue
		}
		t := time.Unix(r.Timestamp, 0).UTC()
		if (!q.From.IsZero() && t.Before(q.From)) || (!q.To.IsZero() && !t.Before(q.To)) {
			continue
		}
		text := strings.ToLower(r.Title + " " + r.Content + " " + r.AuthorName + " " + r.AuthorHandle)
		match := true
		for _, word := range strings.Fields(strings.ToLower(q.Text)) {
			if !strings.Contains(text, word) {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		tokens := Tokens(r.Title + " " + r.Content)
		if q.Term != "" && !tokens[q.Term] {
			continue
		}
		// Division avoids overflow from arbitrarily large page numbers.
		if out.Total/q.PageSize == q.Page-1 {
			out.Results = append(out.Results, r)
		}
		out.Total++
		keywords[r.Keyword]++
		platforms[r.Platform]++
		if r.AuthorKey != "" {
			out.Coverage.KnownAuthors++
			authors[r.AuthorKey]++
			label := r.AuthorHandle
			if label == "" {
				label = r.AuthorName
			}
			if label == "" {
				label = r.AuthorID
			}
			authorLabels[r.AuthorKey] = Count{Key: r.AuthorKey, Label: label, Platform: r.Platform}
		} else {
			out.Coverage.UnknownAuthors++
		}
		if r.Content != "" {
			out.Coverage.WithContent++
		} else {
			out.Coverage.WithoutContent++
		}
		for token := range tokens {
			terms[token]++
		}
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		days[day.Format("2006-01-02")]++
		if first.IsZero() || day.Before(first) {
			first = day
		}
		if last.IsZero() || day.After(last) {
			last = day
		}
	}
	out.Keywords = counts(keywords, 0)
	out.Platforms = counts(platforms, 0)
	out.Terms = counts(terms, 50)
	out.Authors = counts(authors, 0)
	for i, a := range out.Authors {
		label := authorLabels[a.Key]
		label.Count = a.Count
		out.Authors[i] = label
	}
	sort.Slice(out.Authors, func(i, j int) bool {
		a, b := out.Authors[i], out.Authors[j]
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		if a.Platform != b.Platform {
			return a.Platform < b.Platform
		}
		return a.Key < b.Key
	})
	if len(out.Authors) > 20 {
		out.Authors = out.Authors[:20]
	}
	if out.Total > 0 {
		for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
			key := day.Format("2006-01-02")
			out.Activity = append(out.Activity, Count{Key: key, Label: key, Count: days[key]})
		}
	}
	return out
}
