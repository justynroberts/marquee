package nowplaying

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func init() {
	// Keep the other tests off the network.
	fetchLyrics = func(context.Context, Track) ([]lyricLine, error) { return nil, errNoLyrics }
}

const sampleLRC = "[ar: x]\n[00:01.00]\n[00:12.50]Kid A, Kid A\n[00:17.20][01:02.00]Everything in its right place\n[00:25.00]Yesterday I woke up sucking a lemon\n"

func TestParseLRC(t *testing.T) {
	ls := parseLRC(sampleLRC)
	if len(ls) != 5 || ls[0].text != "" || ls[1].t != 12.5 || ls[2].text != "Everything in its right place" || ls[4].t != 62 {
		t.Fatalf("%+v", ls)
	}
	SetLead(0.35)
	defer SetLead(DefaultLead)
	cases := map[float64]int{0: -1, 1: 0, 12.2: 1, 16.9: 2, 24.7: 3, 70: 4}
	for pos, want := range cases {
		if got := lineAt(ls, pos); got != want {
			t.Errorf("lineAt(%v)=%d want %d", pos, got, want)
		}
	}
}

func TestFetchLRCLIBFallsBackToSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/get":
			http.NotFound(w, r)
		case "/search":
			fmt.Fprintf(w, `[{"duration":300,"syncedLyrics":"[00:01.00]wrong length"},{"duration":251,"syncedLyrics":%q},{"duration":251,"syncedLyrics":""}]`, sampleLRC)
		}
	}))
	defer srv.Close()
	old := LyricsBase
	LyricsBase = srv.URL
	defer func() { LyricsBase = old }()
	ls, err := fetchLRCLIB(context.Background(), Track{Title: "Everything In Its Right Place", Artist: "Radiohead", Album: "Kid A", Duration: 251})
	if err != nil || len(ls) != 5 {
		t.Fatalf("%v %+v", err, ls)
	}
	if _, err := fetchLRCLIB(context.Background(), Track{Title: "x", Artist: "y", Duration: 10}); err != errNoLyrics {
		t.Fatalf("want errNoLyrics, got %v", err)
	}
}

func TestLyricsRender(t *testing.T) {
	at := time.Now()
	v := viewWith(Track{Source: "Spotify", Title: "Everything In Its Right Place", Artist: "Radiohead", Album: "Kid A", Position: 17.5, Duration: 251, Playing: true}, synthArt(640), at)
	v.st.lyrics, v.st.lyrGen = parseLRC(sampleLRC), 1
	a := v.Karaoke(at)
	b := v.Karaoke(at.Add(8 * time.Second)) // next line
	if bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("lyric line did not change")
	}
	if p := os.Getenv("NOWPLAYING_PNG"); p != "" {
		var buf bytes.Buffer
		_ = png.Encode(&buf, a)
		_ = os.WriteFile(p, buf.Bytes(), 0o644)
	}
}

// NOWPLAYING_REAL=1 go test -run TestRealLyrics -v
func TestRealLyrics(t *testing.T) {
	if os.Getenv("NOWPLAYING_REAL") == "" {
		t.Skip("set NOWPLAYING_REAL=1")
	}
	ls, err := fetchLRCLIB(context.Background(), Track{Title: "Everything In Its Right Place", Artist: "Radiohead", Album: "Kid A", Duration: 251})
	t.Logf("err=%v lines=%d first=%+v", err, len(ls), ls[:min(3, len(ls))])
}

func TestLongLyricWrapsNotTruncated(t *testing.T) {
	long := "And I'll be the one who takes the blame when the whole thing falls apart tonight"
	ls, px := layoutLyric(long, float64(tw))
	if len(ls) != 2 {
		t.Fatalf("want two lines, got %d at %dpx: %q", len(ls), px, ls)
	}
	for _, l := range ls {
		if strings.Contains(l, "…") || measureBold(l, px) > float64(tw) {
			t.Fatalf("truncated or too wide: %q", l)
		}
	}
	if strings.Join(ls, " ") != long {
		t.Fatalf("words lost: %q", ls)
	}
	if os.Getenv("NOWPLAYING_PNG") != "" {
		at := time.Now()
		v := viewWith(Track{Source: "Spotify", Title: "What You Want", Artist: "Ren", Position: 17.5, Duration: 196, Playing: true}, synthArt(640), at)
		v.st.lyrics, v.st.lyrGen = []lyricLine{{0, long}, {30, "I got what you need"}}, 1
		var buf bytes.Buffer
		_ = png.Encode(&buf, v.Karaoke(at.Add(1500*time.Millisecond)))
		_ = os.WriteFile(strings.TrimSuffix(os.Getenv("NOWPLAYING_PNG"), ".png")+"-two.png", buf.Bytes(), 0o644)
	}
}

func TestStaticIgnoresLyricsAndSweepMoves(t *testing.T) {
	at := time.Now()
	v := viewWith(Track{Source: "Spotify", Title: "T", Artist: "A", Position: 12.6, Duration: 200, Playing: true}, nil, at)
	v.st.lyrics, v.st.lyrGen = parseLRC(sampleLRC), 1
	static := v.Frame(at)
	plain := viewWith(Track{Source: "Spotify", Title: "T", Artist: "A", Position: 12.6, Duration: 200, Playing: true}, nil, at)
	if !bytes.Equal(static.Pix, plain.Frame(at).Pix) {
		t.Fatal("static view drew lyrics")
	}
	k1 := append([]byte(nil), v.Karaoke(at).Pix...)
	k2 := v.Karaoke(at.Add(1500 * time.Millisecond)).Pix
	if bytes.Equal(k1, k2) {
		t.Fatal("karaoke sweep did not move within a line")
	}
	cur, next, sw, ok := v.Lyric(at.Add(1500 * time.Millisecond))
	if !ok || cur != "Kid A, Kid A" || next != "Everything in its right place" || sw <= 0 || sw >= 1 {
		t.Fatalf("Lyric: %q %q %v %v", cur, next, sw, ok)
	}
}
