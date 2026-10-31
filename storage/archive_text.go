package storage

import (
	"html"
	"regexp"
	"strings"
)

var archiveMarkup = regexp.MustCompile(`(?s)<[^>]*>`)
var archiveScripts = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</(?:script|style)\s*>`)

// ArchiveContent converts only provider fields stored as HTML to plain text.
// Other providers supply plain text (Fediverse is cleaned by its collector).
func ArchiveContent(platform, content string) string {
	if platform == "HackerNews" {
		return ArchivePlainText(content)
	}
	return content
}

// ArchivePlainText converts known HTML to text and keeps adjacent blocks separate.
// Do not apply it to plain text: literal angle brackets are not markup.
func ArchivePlainText(s string) string {
	s = archiveScripts.ReplaceAllString(s, " ")
	s = archiveMarkup.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(html.UnescapeString(s)), " ")
}
