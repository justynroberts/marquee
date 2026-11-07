package nowplaying

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"
)

func synthArt(n int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, n, n*3/4+n/4)) // square-ish
	for y := 0; y < img.Rect.Dy(); y++ {
		for x := 0; x < n; x++ {
			c := color.RGBA{uint8(40 + x*140/n), uint8(60 + y*60/n), 90, 255}
			if (x/(n/8)+y/(n/8))%2 == 0 {
				c = color.RGBA{200, 120, 60, 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func viewWith(t Track, art image.Image, at time.Time) *View {
	v := newView()
	v.st.track, v.st.polledAt = &t, at
	if art != nil {
		v.st.art = squareArt(art, side)
		v.st.artGen = 1
	}
	return v
}

func bright(img *image.RGBA, r image.Rectangle) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if img.Pix[img.PixOffset(x, y)] > 150 {
				n++
			}
		}
	}
	return n
}

func TestIdle(t *testing.T) {
	v := newView()
	img := v.Frame(time.Now())
	if img.Bounds() != image.Rect(0, 0, W, H) {
		t.Fatalf("size %v", img.Bounds())
	}
	if img.Pix[0] != bg.R || img.Pix[1] != bg.G {
		t.Error("background")
	}
	if n := bright(img, img.Rect); n < 500 {
		t.Errorf("idle text missing: %d bright px", n)
	}
	if v.Frame(time.Now().Add(time.Second)) != img {
		t.Error("idle frame should be cached")
	}
	// Permission message replaces the hint.
	v.apply(poll{Errs: []playerError{{Source: "Music", Code: -1743, Msg: "Not authorized"}}}, time.Now())
	if v.st.msg != "Allow Marquee to control Music in System Settings › Privacy & Security › Automation" {
		t.Errorf("msg %q", v.st.msg)
	}
	if v.Frame(time.Now()) == img {
		t.Error("message change should redraw")
	}
}

func TestTrackWithArtwork(t *testing.T) {
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	v := viewWith(Track{Source: "Music", Title: "Short", Artist: "Artist", Album: "Album", Position: 30, Duration: 120, Playing: true}, synthArt(600), at)
	start := time.Now()
	img := v.Frame(at)
	if el := time.Since(start); el > 200*time.Millisecond {
		t.Errorf("first frame took %s", el)
	}
	// Artwork occupies the left square: not background.
	c := img.RGBAAt(pad+side/2, pad+side/2)
	if c == bg {
		t.Error("artwork not drawn")
	}
	if n := bright(img, image.Rect(tx, titleY, W, titleY+titleH)); n < 1000 {
		t.Errorf("title missing (%d)", n)
	}
	// Progress interpolates while playing: 30 s + 30 s = halfway.
	img2 := v.Frame(at.Add(30 * time.Second))
	if img2 == img {
		t.Fatal("frame should change with position")
	}
	mid := tx + tw/2
	if img2.RGBAAt(mid-10, barY+2) != ink || img2.RGBAAt(mid+20, barY+2) != rule {
		t.Error("progress bar not at halfway")
	}
	// Same second, same bar pixel: cached.
	if v.Frame(at.Add(30*time.Second+time.Millisecond)) != img2 {
		t.Error("unchanged frame should be cached")
	}
	// Paused: no interpolation.
	v.st.track.Playing = false
	p := v.Frame(at.Add(90 * time.Second))
	if p.RGBAAt(tx+tw/4+20, barY+2) != rule {
		t.Error("paused position moved")
	}
}

func TestNoArtwork(t *testing.T) {
	v := viewWith(Track{Source: "Spotify", Title: "x", Playing: false}, nil, time.Now())
	img := v.Frame(time.Now())
	if img.RGBAAt(pad+10, pad+10) != rule {
		t.Error("placeholder missing")
	}
}

func TestTitleScrolls(t *testing.T) {
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	v := viewWith(Track{Source: "Spotify", Title: strings.Repeat("A very long song title ", 3), Artist: "B", Duration: 200, Playing: false}, nil, at)
	a := v.Frame(at)
	if v.r.strip == nil {
		t.Fatal("long title should scroll")
	}
	region := image.Rect(tx, titleY, W-pad, titleY+titleH)
	snap := func(img *image.RGBA) []byte {
		var b []byte
		for y := region.Min.Y; y < region.Max.Y; y++ {
			b = append(b, img.Pix[img.PixOffset(region.Min.X, y):img.PixOffset(region.Max.X, y)]...)
		}
		return b
	}
	sa := snap(a)
	if !bytes.Equal(sa, snap(v.Frame(at.Add(time.Second)))) {
		t.Error("title should hold during the first pass")
	}
	b := v.Frame(at.Add(4 * time.Second))
	if bytes.Equal(sa, snap(b)) {
		t.Error("title did not move")
	}
	// After a full cycle it is back at rest.
	cycle := scrollHold + float64(v.r.period)/scrollSpeed
	c := v.Frame(at.Add(time.Duration((cycle + 0.5) * float64(time.Second))))
	if !bytes.Equal(sa, snap(c)) {
		t.Error("title should loop back to the start")
	}
	// The scroll never paints over the artwork.
	if a.RGBAAt(pad+side-2, titleY+50) != rule {
		t.Error("strip overlapped artwork")
	}
}

func TestParse(t *testing.T) {
	us, rs := "\x1f", "\x1e"
	out := strings.Join([]string{
		"T" + us + "Music" + us + "paused" + us + "Title, with | pipes" + us + "Art" + us + "Alb" + us + "12500" + us + "240000" + us + "" + us + "ABC123",
		"T" + us + "Spotify" + us + "playing" + us + "S" + us + "" + us + "" + us + "61234" + us + "180000" + us + "https://i.scdn.co/image/x" + us + "spotify:track:1",
		"E" + us + "Music" + us + "-1743" + us + "Not authorized to send Apple events to Music.",
		"garbage",
		"T" + us + "short",
	}, rs) + rs + "\n"
	p := parse(out)
	if len(p.Tracks) != 2 || len(p.Errs) != 1 {
		t.Fatalf("got %+v", p)
	}
	m := p.Tracks[0]
	if m.Title != "Title, with | pipes" || m.Position != 12.5 || m.Duration != 240 || m.Playing || m.ArtURL != "" || m.ID != "ABC123" {
		t.Errorf("music %+v", m)
	}
	s := p.Tracks[1]
	if s.Artist != "" || s.Album != "" || s.ArtURL == "" || !s.Playing || s.Position != 61.234 {
		t.Errorf("spotify %+v", s)
	}
	if got, _ := p.pick(); got.Source != "Spotify" {
		t.Error("playing track should win over paused")
	}
	if !p.Errs[0].denied() {
		t.Error("-1743 is a permission error")
	}
	if p := parse(""); len(p.Tracks)+len(p.Errs) != 0 {
		t.Error("empty output")
	}
	if _, ok := parse("").pick(); ok {
		t.Error("nothing to pick")
	}
	// Spotify builds that report position in ms.
	q := parse("T" + us + "Spotify" + us + "playing" + us + "S" + us + "a" + us + "b" + us + "61234000" + us + "180000" + us + "" + us + "id")
	if q.Tracks[0].Position != 61.234 {
		t.Errorf("ms position %v", q.Tracks[0].Position)
	}
}

func TestPollerInjected(t *testing.T) {
	oldF, oldA := fetch, fetchArt
	defer func() { fetch, fetchArt = oldF, oldA }()
	fetch = func(context.Context) (poll, error) {
		return poll{Tracks: []Track{{Source: "Music", ID: "1", Title: "T", Artist: "A", Duration: 100, Position: 5, Playing: true}}}, nil
	}
	got := make(chan struct{}, 1)
	fetchArt = func(context.Context, Track) (image.Image, error) {
		select {
		case got <- struct{}{}:
		default:
		}
		return synthArt(300), nil
	}
	v := New()
	defer v.Close()
	select {
	case <-got:
	case <-time.After(3 * time.Second):
		t.Fatal("artwork never fetched")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		v.mu.Lock()
		ok := v.st.art != nil
		v.mu.Unlock()
		if ok {
			img := v.Frame(time.Now())
			if img.RGBAAt(pad+side/2, pad+side/2) == bg {
				t.Error("artwork not rendered")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("artwork never applied")
}

// NOWPLAYING_PNG=/path/out.png go test -run TestSamplePNG ./internal/nowplaying
// writes a sample frame (plus -idle and -denied variants) for eyeballing.
func TestSamplePNG(t *testing.T) {
	path := os.Getenv("NOWPLAYING_PNG")
	if path == "" {
		t.Skip("set NOWPLAYING_PNG")
	}
	at := time.Now()
	v := viewWith(Track{Source: "Music", Title: "Everything In Its Right Place", Artist: "Radiohead", Album: "Kid A", Position: 151, Duration: 251, Playing: true}, synthArt(640), at)
	save := func(p string, img *image.RGBA) {
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("wrote", p)
	}
	save(path, v.Frame(at.Add(3500*time.Millisecond)))
	base := strings.TrimSuffix(path, ".png")
	pv := viewWith(Track{Source: "Spotify", Title: "Short title", Artist: "Someone", Album: "Something", Position: 42, Duration: 3700, Playing: false}, nil, at)
	save(base+"-paused.png", pv.Frame(at))
	save(base+"-idle.png", newView().Frame(at))
	dv := newView()
	dv.apply(poll{Errs: []playerError{{Source: "Music", Code: -1743}}}, at)
	save(base+"-denied.png", dv.Frame(at))
}

// NOWPLAYING_REAL=1 go test -run TestRealFetch -v ./internal/nowplaying
func TestRealFetch(t *testing.T) {
	if os.Getenv("NOWPLAYING_REAL") == "" {
		t.Skip("set NOWPLAYING_REAL=1")
	}
	p, err := fetchOsascript(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("tracks=%+v errs=%+v", p.Tracks, p.Errs)
	if tr, ok := p.pick(); ok {
		img, err := fetchArtwork(context.Background(), tr)
		t.Logf("artwork: img=%v err=%v", img != nil, err)
	} else {
		t.Logf("nothing playing; idle message %q", p.idleMessage())
	}
}
