package dashboard

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// onlineFrame polls a view until its background fetch has landed, then returns a frame.
func onlineFrame(t *testing.T, name string, frame func() *image.RGBA, ready func() bool) *image.RGBA {
	t.Helper()
	frame()
	for i := 0; i < 200 && !ready(); i++ {
		time.Sleep(5 * time.Millisecond)
	}
	if !ready() {
		t.Fatalf("%s: fetch never landed", name)
	}
	img := frame()
	lit := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if max(img.Pix[i], img.Pix[i+1], img.Pix[i+2]) > 120 {
			lit++
		}
	}
	if lit < 4000 {
		t.Errorf("%s: only %d lit pixels", name, lit)
	}
	if dir := os.Getenv("HIRES_PNG"); dir != "" {
		f, _ := os.Create(filepath.Join(dir, name+".png"))
		png.Encode(f, img)
		f.Close()
	}
	return img
}

func pollReady[T any](p *poller[T]) func() bool {
	return func() bool { p.mu.Lock(); defer p.mu.Unlock(); return p.ok }
}

func TestOnlineRain(t *testing.T) {
	now := time.Date(2026, 11, 10, 14, 7, 0, 0, time.UTC)
	start := now.Truncate(15 * time.Minute)
	for _, tc := range []struct {
		name string
		mm   []float64
		want string
	}{
		{"rain", []float64{0, 0, 0.2, 0.6, 1.4, 0.8, 0.1, 0}, "RAIN FROM 14:30"},
		{"rain-now", []float64{0.4, 0.5, 0.2, 0, 0, 0, 0, 0}, "RAIN UNTIL 14:45"},
		{"rain-dry", []float64{0, 0, 0, 0, 0, 0, 0, 0}, "DRY UNTIL 19:00"},
	} {
		rc := &feeds.RainCast{Place: "Westminster"}
		for i, mm := range tc.mm {
			rc.Quarters = append(rc.Quarters, feeds.RainSlot{At: start.Add(time.Duration(i) * 15 * time.Minute), MM: mm, Chance: int(mm * 60)})
		}
		for i := range 12 {
			h := feeds.RainSlot{At: now.Truncate(time.Hour).Add(time.Duration(i) * time.Hour), Chance: (i * 9) % 90}
			if i == 5 || (tc.name != "rain-dry" && i == 1) {
				h.MM = 1.2
			}
			rc.Hours = append(rc.Hours, h)
		}
		if got, _ := rainVerdict(rc); got != tc.want {
			t.Errorf("%s: verdict %q, want %q", tc.name, got, tc.want)
		}
		r := NewRain(func() string { return "SW1A" })
		r.fetch = func(string) (*feeds.RainCast, error) { return rc, nil }
		onlineFrame(t, tc.name, func() *image.RGBA { return r.Frame(0, 0, now) }, pollReady(&r.poll))
	}
}

func TestOnlineStatus(t *testing.T) {
	pages, _ := ParseStatus(append(DefaultStatus, "npm", "Example = status.example.com"))
	s := NewStatus(func() []feeds.StatusPage { return pages })
	s.fetch = func(p feeds.StatusPage) (*feeds.ServiceStatus, error) {
		switch p.Name {
		case "Cloudflare":
			return &feeds.ServiceStatus{Name: p.Name, Indicator: "minor", Description: "Minor Service Outage",
				Down:      []feeds.Component{{Name: "Dashboard", Status: "degraded_performance"}},
				Incidents: []feeds.StatusIncident{{Name: "Elevated errors on the dashboard and API in some regions", Impact: "minor", Status: "investigating", Update: "We are investigating reports of elevated error rates."}}}, nil
		case "OpenAI":
			return &feeds.ServiceStatus{Name: p.Name, Indicator: "none", Down: []feeds.Component{{Name: "Sora", Status: "partial_outage"}}}, nil
		case "Example":
			return nil, fmt.Errorf("HTTP 404")
		}
		return &feeds.ServiceStatus{Name: p.Name, Indicator: "none", Description: "All Systems Operational"}, nil
	}
	onlineFrame(t, "status", func() *image.RGBA { return s.Frame(0, 0, time.Now()) }, pollReady(&s.poll))
}

func TestParseStatus(t *testing.T) {
	p, bad := ParseStatus([]string{"# c", "", "GitHub", "Acme = https://status.acme.io/", "nope", "Bad = ftp://x", "Mine = status.mine.dev"})
	if len(p) != 3 || p[0].URL != "https://www.githubstatus.com" || p[1].URL != "https://status.acme.io" || p[2].URL != "https://status.mine.dev" {
		t.Fatalf("%+v", p)
	}
	if strings.Join(bad, "|") != "nope|Bad = ftp://x" {
		t.Fatalf("bad %v", bad)
	}
	if got := LoadStatus(filepath.Join(t.TempDir(), "none.txt")); len(got) != len(DefaultStatus) {
		t.Fatalf("defaults %v", got)
	}
}

func TestOnlineLaunches(t *testing.T) {
	now := time.Date(2026, 11, 10, 14, 7, 0, 0, time.UTC)
	ls := []feeds.Launch{
		{Rocket: "Falcon 9 Block 5", Mission: "Starlink Group 10-12", Orbit: "LEO", Provider: "SpaceX", Location: "Cape Canaveral SFS, FL, USA", Country: "USA", Net: now.Add(26*time.Hour + 3*time.Minute + 9*time.Second), Precision: "Second", Status: "Go"},
		{Rocket: "Long March 2D", Mission: "Unknown Payload", Provider: "CASC", Country: "China", Net: now.Add(40 * time.Hour), Precision: "Hour", Status: "TBC"},
		{Rocket: "Electron", Mission: "Owl For One", Provider: "Rocket Lab", Country: "New Zealand", Net: now.Add(80 * time.Hour), Precision: "Day", Status: "TBD"},
		{Rocket: "Ariane 6", Mission: "Galileo L14", Provider: "Arianespace", Country: "French Guiana", Net: now.Add(300 * time.Hour), Precision: "Month", Status: "TBD"},
	}
	l := NewLaunches()
	l.fetch = func() ([]feeds.Launch, error) { return ls, nil }
	onlineFrame(t, "launches", func() *image.RGBA { return l.Frame(0, 0, now) }, pollReady(&l.poll))
	if got := lnCountdown(26*time.Hour + 3*time.Minute + 9*time.Second); got != "T- 1D 02:03:09" {
		t.Errorf("countdown %q", got)
	}
	if got := lnCountdown(-75 * time.Second); got != "T+ 00:01:15" {
		t.Errorf("countdown %q", got)
	}
}

func TestOnlineFX(t *testing.T) {
	cfg := DefaultFX
	f := NewFX(func() FXConfig { return cfg })
	f.fetch = func(base string, quotes []string) ([]feeds.Rate, error) {
		var out []feeds.Rate
		for i, q := range quotes {
			r := feeds.Rate{Code: q, Date: "2026-11-10"}
			v := []float64{1.34, 1.19, 204.5, 1.07, 2.05, 1.88}[i]
			for d := range 22 {
				r.Month = append(r.Month, v*(1+0.01*float64((d*7+i*3)%11-5)/5))
			}
			r.Rate, r.Prev = r.Month[21], r.Month[20]
			out = append(out, r)
		}
		return out, nil
	}
	onlineFrame(t, "fx", func() *image.RGBA { return f.Frame(0, 0, time.Now()) }, pollReady(&f.poll))
}

func TestParseFX(t *testing.T) {
	c, err := ParseFX([]string{"# rates", "gbp: usd, eur jpy usd"})
	if err != nil || c.Base != "GBP" || strings.Join(c.Quotes, " ") != "USD EUR JPY" {
		t.Fatalf("%+v %v", c, err)
	}
	for _, bad := range []string{"GBP", "GBP DOLLARS", "GBP A B C D E F G", "GBP USD EUR JPY CHF AUD CAD NZD"} {
		if _, err := ParseFX([]string{bad}); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if c := LoadFX(filepath.Join(t.TempDir(), "none.txt")); c.Base != "GBP" || len(c.Quotes) != 6 {
		t.Fatalf("%+v", c)
	}
}
