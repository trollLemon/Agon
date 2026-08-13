package archive

import (
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	summaryLimit    = 80
	titleTrimCutset = " ,;:—-"
)

var (
	contextHeadRe = regexp.MustCompile(`(?i)\b(context|constraints?|background|notes?)\s*:`)
	slugNonAlnum  = regexp.MustCompile(`[^a-z0-9]+`)
)

// SummarizeTitle derives a compact human label from a full proposition +
// context blob: it drops any trailing context section, keeps the first
// sentence, and truncates on a word boundary.
func SummarizeTitle(topic string) string {
	text := strings.Join(strings.Fields(topic), " ")
	if text == "" {
		return "Debate"
	}
	if loc := contextHeadRe.FindStringIndex(text); loc != nil {
		text = strings.TrimSpace(text[:loc[0]])
	}
	text = firstSentence(text)
	if text == "" {
		return "Debate"
	}
	if utf8.RuneCountInString(text) <= summaryLimit {
		return text
	}
	runes := []rune(text)
	cut := strings.TrimRight(string(runes[:summaryLimit]), titleTrimCutset)
	if i := strings.LastIndex(cut, " "); i >= 0 {
		cut = strings.TrimRight(cut[:i], titleTrimCutset)
	}
	if cut == "" {
		cut = strings.TrimSpace(string(runes[:summaryLimit]))
	}
	return cut + "…"
}

// firstSentence returns the leading sentence, cutting at the first
// sentence-ending punctuation that is followed by whitespace.
func firstSentence(s string) string {
	runes := []rune(s)
	for i := 0; i+1 < len(runes); i++ {
		switch runes[i] {
		case '.', '?', '!':
			if unicode.IsSpace(runes[i+1]) {
				return strings.TrimSpace(string(runes[:i+1]))
			}
		}
	}
	return strings.TrimSpace(s)
}

// NewSessionID builds a timestamp-based session id from a topic: a kebab-case
// slug of its summarized title, followed by a compact UTC timestamp, e.g.
// "adopt-event-sourcing-20260812-201500".
func NewSessionID(topic string, now time.Time) string {
	slug := slugify(SummarizeTitle(topic))
	if slug == "" {
		slug = "debate"
	}
	return slug + "-" + now.UTC().Format("20060102-150405")
}

// slugify lower-cases s and collapses everything but letters/digits into
// single hyphens, trimming leading/trailing hyphens.
func slugify(s string) string {
	lower := strings.ToLower(s)
	slug := slugNonAlnum.ReplaceAllString(lower, "-")
	slug = strings.Trim(slug, "-")
	const maxSlugLen = 60
	if len(slug) > maxSlugLen {
		slug = strings.Trim(slug[:maxSlugLen], "-")
	}
	return slug
}
