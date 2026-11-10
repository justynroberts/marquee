package feeds

import (
	"context"
	"strings"
)

// Source is one named news source: Hacker News ("hn") or an RSS/Atom feed.
type Source struct {
	Key   string // preset name, or "" for a custom URL
	Label string // shown on the panel
	URL   string // "hn" for Hacker News
}

// Presets are the sources that can be named instead of given as a URL.
var Presets = []Source{
	{"bbc", "BBC News", "https://feeds.bbci.co.uk/news/rss.xml"},
	{"bbc-world", "BBC World", "https://feeds.bbci.co.uk/news/world/rss.xml"},
	{"bbc-tech", "BBC Technology", "https://feeds.bbci.co.uk/news/technology/rss.xml"},
	{"bbc-sport", "BBC Sport", "https://feeds.bbci.co.uk/sport/rss.xml"},
	{"sky", "Sky News", "https://feeds.skynews.com/feeds/rss/home.xml"},
	{"guardian", "The Guardian", "https://www.theguardian.com/uk/rss"},
	{"aljazeera", "Al Jazeera", "https://www.aljazeera.com/xml/rss/all.xml"},
	{"npr", "NPR", "https://feeds.npr.org/1001/rss.xml"},
	{"hn", "Hacker News", "hn"},
	{"verge", "The Verge", "https://www.theverge.com/rss/index.xml"},
	{"ars", "Ars Technica", "https://feeds.arstechnica.com/arstechnica/index"},
}

// LookupSource resolves a preset name or an http(s) feed URL; ok is false for anything else.
func LookupSource(s string) (Source, bool) {
	s = strings.TrimSpace(s)
	low := strings.ToLower(s)
	if low == "hackernews" {
		low = "hn"
	}
	for _, p := range Presets {
		if p.Key == low {
			return p, true
		}
	}
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return Source{URL: s}, true
	}
	return Source{}, false
}

// Fetch returns up to n headlines from the source. A custom feed's label becomes the feed's
// own title.
func (s Source) Fetch(ctx context.Context, n int) ([]Headline, error) {
	if s.URL == "hn" {
		return FetchHN(ctx, n)
	}
	return FetchRSS(ctx, s.URL, n)
}
