package feeds

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Headline is one news item.
type Headline struct {
	Source string
	Title  string
	Points int // Hacker News score; 0 for RSS
}

var newsClient = &http.Client{Timeout: 10 * time.Second}

func getBody(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Marquee (+https://github.com/justynroberts/marquee)")
	resp, err := newsClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// HNBase is the Hacker News API root (tests point it elsewhere).
var HNBase = "https://hacker-news.firebaseio.com/v0"

// FetchHN returns the current top n Hacker News stories, in rank order.
func FetchHN(ctx context.Context, n int) ([]Headline, error) {
	b, err := getBody(ctx, HNBase+"/topstories.json", 1<<20)
	if err != nil {
		return nil, err
	}
	var ids []int
	if err := json.Unmarshal(b, &ids); err != nil {
		return nil, fmt.Errorf("hacker news: %v", err)
	}
	ids = ids[:min(n, len(ids))]
	out := make([]Headline, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, err := getBody(ctx, fmt.Sprintf("%s/item/%d.json", HNBase, id), 1<<16)
			if err != nil {
				return
			}
			var it struct {
				Title string `json:"title"`
				Score int    `json:"score"`
			}
			if json.Unmarshal(b, &it) == nil {
				out[i] = Headline{Source: "HN", Title: it.Title, Points: it.Score}
			}
		}()
	}
	wg.Wait()
	var res []Headline
	for _, h := range out {
		if h.Title != "" {
			res = append(res, h)
		}
	}
	if len(res) == 0 {
		return nil, fmt.Errorf("hacker news: no stories")
	}
	return res, nil
}

// FetchRSS reads an RSS 2.0 or Atom feed and returns its first n item titles. The source label
// is the feed's own title.
func FetchRSS(ctx context.Context, url string, n int) ([]Headline, error) {
	b, err := getBody(ctx, url, 4<<20)
	if err != nil {
		return nil, err
	}
	return ParseFeed(b, n)
}

// ParseFeed parses RSS 2.0 or Atom. It reads the feed as XML first; only when that finds
// nothing does it retry with HTML void elements auto-closed, for sloppy feeds. (Auto-closing
// from the start breaks RSS, whose <link> has content.)
func ParseFeed(b []byte, n int) ([]Headline, error) {
	out, err := parseFeed(b, n, false)
	if err != nil || len(out) == 0 {
		if o2, err2 := parseFeed(b, n, true); err2 == nil && len(o2) > 0 {
			return o2, nil
		}
	}
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("feed: no items")
	}
	return out, nil
}

func parseFeed(b []byte, n int, autoClose bool) ([]Headline, error) {
	var doc struct {
		XMLName xml.Name
		Channel struct {
			Title string `xml:"title"`
			Items []struct {
				Title string `xml:"title"`
			} `xml:"item"`
		} `xml:"channel"`
		Title   string `xml:"title"`
		Entries []struct {
			Title string `xml:"title"`
		} `xml:"entry"`
	}
	dec := xml.NewDecoder(strings.NewReader(string(b)))
	dec.Strict = false
	if autoClose {
		dec.AutoClose = xml.HTMLAutoClose
	}
	dec.Entity = xml.HTMLEntity
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("feed: %v", err)
	}
	source := strings.TrimSpace(doc.Channel.Title)
	var titles []string
	for _, it := range doc.Channel.Items {
		titles = append(titles, it.Title)
	}
	if doc.XMLName.Local == "feed" {
		source = strings.TrimSpace(doc.Title)
		titles = titles[:0]
		for _, e := range doc.Entries {
			titles = append(titles, e.Title)
		}
	}
	var out []Headline
	for _, t := range titles {
		t = strings.Join(strings.Fields(html.UnescapeString(t)), " ")
		if t != "" {
			out = append(out, Headline{Source: source, Title: t})
		}
		if len(out) == n {
			break
		}
	}
	return out, nil
}

// BoardText turns headlines into message-board markup: one line per headline, the source in
// the accent colour, the title in white, the score dimmed. Board markup in titles is defused.
func BoardText(hs []Headline) string {
	var b strings.Builder
	b.WriteString("@step=4\n@size=30\n")
	for _, h := range hs {
		src := plain(h.Source)
		if len([]rune(src)) > 24 {
			src = string([]rune(src)[:24])
		}
		fmt.Fprintf(&b, "[orange]%s[/] [white]%s[/]", src, plain(h.Title))
		if h.Points > 0 {
			fmt.Fprintf(&b, " [grey]%d points[/]", h.Points)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// plain removes characters the board would read as markup: [tags], {tokens}, :sprites: and a
// leading ! or @.
func plain(s string) string {
	r := strings.NewReplacer("[", "(", "]", ")", "{", "(", "}", ")", "\n", " ", "\r", " ")
	s = r.Replace(s)
	// ":word:" would become a sprite; separate the closing colon.
	var out []rune
	rs := []rune(s)
	for i, c := range rs {
		out = append(out, c)
		if c == ':' && i+1 < len(rs) && rs[i+1] >= 'a' && rs[i+1] <= 'z' {
			out = append(out, ' ')
		}
	}
	return strings.TrimLeft(strings.TrimSpace(string(out)), "!@")
}
