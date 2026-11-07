package feeds

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseFeed(t *testing.T) {
	rss := `<?xml version="1.0"?><rss><channel><title>The Verge</title><item><title>One &amp; two</title></item><item><title><![CDATA[Three  
 four]]></title></item></channel></rss>`
	hs, err := ParseFeed([]byte(rss), 10)
	if err != nil || len(hs) != 2 || hs[0].Title != "One & two" || hs[1].Title != "Three four" || hs[0].Source != "The Verge" {
		t.Fatalf("rss: %v %+v", err, hs)
	}
	atom := `<feed xmlns="http://www.w3.org/2005/Atom"><title>Blog</title><entry><title>A</title></entry><entry><title>B</title></entry><entry><title>C</title></entry></feed>`
	hs, err = ParseFeed([]byte(atom), 2)
	if err != nil || len(hs) != 2 || hs[1].Title != "B" || hs[0].Source != "Blog" {
		t.Fatalf("atom: %v %+v", err, hs)
	}
}

func TestBoardTextDefusesMarkup(t *testing.T) {
	out := BoardText([]Headline{{Source: "HN", Title: "![big] {time} :fire: wins", Points: 42}})
	for _, bad := range []string{"[big]", "{time}", ":fire:"} {
		if strings.Contains(out, bad) {
			t.Fatalf("markup %q survived: %q", bad, out)
		}
	}
	if !strings.Contains(out, "42 points") || !strings.HasPrefix(out, "@step=4\n@size=30\n[orange]HN[/]") {
		t.Fatalf("format: %q", out)
	}
}

func TestFetchHN(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/topstories.json" {
			fmt.Fprint(w, "[3,1,2]")
			return
		}
		var id int
		fmt.Sscanf(r.URL.Path, "/item/%d.json", &id)
		fmt.Fprintf(w, `{"title":"story %d","score":%d}`, id, id*10)
	}))
	defer srv.Close()
	old := HNBase
	HNBase = srv.URL
	defer func() { HNBase = old }()
	hs, err := FetchHN(context.Background(), 2)
	if err != nil || len(hs) != 2 || hs[0].Title != "story 3" || hs[1].Points != 10 {
		t.Fatalf("%v %+v", err, hs)
	}
}
