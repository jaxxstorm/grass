package storage

import (
	"html"
	"regexp"
	"strings"
)

var archiveMarkup = regexp.MustCompile(`(?s)<[^>]*>`)
var archiveScripts = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</(?:script|style)\s*>`)

// ArchivePlainText makes source markup inert and keeps adjacent blocks separate.
func ArchivePlainText(s string) string {
	s = archiveScripts.ReplaceAllString(s, " ")
	s = archiveMarkup.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(html.UnescapeString(s)), " ")
}
