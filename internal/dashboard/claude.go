package dashboard

import (
	"fmt"
	"image"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// ClaudeCode is Claude Code's use on this Mac, read from its own transcripts: today's tokens by
// kind, output by the hour, the last seven days, and the sessions running now with how full
// each conversation is. Nothing leaves the Mac.
type ClaudeCode struct {
	scan  func(time.Time) (*feeds.ClaudeUsage, error)
	poll  poller[*feeds.ClaudeUsage]
	cache frameCache
	busy  [][2]float64 // dots to pulse for working sessions
}

var (
	ccOnce    sync.Once
	ccScanner *feeds.ClaudeScanner
)

// NewClaudeCode shares one scanner across plays, so only the first reads the whole week.
func NewClaudeCode() *ClaudeCode {
	ccOnce.Do(func() { ccScanner = feeds.NewClaudeScanner(feeds.ClaudeHome()) })
	return &ClaudeCode{scan: ccScanner.Scan, poll: poller[*feeds.ClaudeUsage]{every: 20 * time.Second}}
}

func (c *ClaudeCode) Frame(t, dt float64, now time.Time) *image.RGBA {
	u, got, ok, err := c.poll.get("", func(string) (*feeds.ClaudeUsage, error) { return c.scan(time.Now()) })
	if !ok {
		return waitMessage(&c.cache, "CLAUDE CODE USAGE", err)
	}
	img := c.cache.get(fmt.Sprint(got.UnixNano(), now.Format("15:04")), func(img *image.RGBA) { c.render(img, u, now) })
	k := 0.5 + 0.5*math.Sin(t*4)
	for _, p := range c.busy {
		ring(img, int(p[0]), int(p[1]), 9+5*k, mix(bg, amber, 1-k*0.7))
	}
	return img
}

// tok formats a token count: 950, 12.3K, 4.56M, 1.2B.
func tok(n int64) string {
	f := float64(n)
	switch {
	case n < 1000:
		return fmt.Sprint(n)
	case n < 1e6:
		return fmt.Sprintf("%.1fK", f/1e3)
	case n < 1e9:
		return fmt.Sprintf("%.2fM", f/1e6)
	}
	return fmt.Sprintf("%.2fB", f/1e9)
}

const (
	ccX1 = 560  // hours chart
	ccX2 = 1230 // sessions
)

func (c *ClaudeCode) render(img *image.RGBA, u *feeds.ClaudeUsage, now time.Time) {
	head(img, pad, pad-4, "claude code  today", ccX1-pad-40)
	text(img, pad, 74, tok(u.Today.Out), 84, amber)
	text(img, pad, 172, "OUTPUT TOKENS", 20, txt2)
	ccLogo(img, ccX1-90, 116, 40, 0.3, claudeOrange)
	rows := []struct {
		k, v string
	}{
		{"INPUT", tok(u.Today.In)},
		{"CACHE WRITE", tok(u.Today.CacheWrite)},
		{"CACHE READ", tok(u.Today.CacheRead)},
		{"CACHE HITS", fmt.Sprintf("%.0f%%", u.Today.CacheHit()*100)},
		{"API CALLS", fmt.Sprint(u.Today.Calls)},
	}
	for i, r := range rows {
		y := 214 + i*36
		text(img, pad, y, r.k, 22, cyan)
		text(img, pad+220, y, r.v, 22, txt)
	}
	text(img, pad, 410, "FROM THIS MAC'S CLAUDE CODE HISTORY", 16, txt3)
	vrule(img, ccX1-30)

	// output by the hour, then the week
	x0, x1 := ccX1, ccX2-50
	head(img, x0, pad-4, "output by hour", x1-x0)
	var top int64 = 1
	for _, h := range u.Hours {
		top = max(top, h.Out)
	}
	cy0, cy1 := 80, 262
	step := float64(x1-x0) / 24
	for h, v := range u.Hours {
		bx := x0 + int(float64(h)*step)
		c := violet
		if h == now.Hour() {
			c = amber
		}
		bh := int(float64(v.Out) / float64(top) * float64(cy1-cy0))
		if v.Out > 0 {
			rect(img, bx, cy1-max(bh, 2), bx+int(step)-4, cy1, c)
		}
		if h%3 == 0 {
			text(img, float64(bx), cy1+6, fmt.Sprintf("%02d", h), 16, txt3)
		}
	}
	text(img, float64(x1)-measure(tok(top), 16), pad, tok(top), 16, txt3)

	head(img, x0, 300, "last 7 days  output", x1-x0)
	var wk int64 = 1
	var sum int64
	for _, d := range u.Days {
		wk = max(wk, d.Out)
		sum += d.Out
	}
	text(img, float64(x1)-measure(tok(sum), 18), 302, tok(sum), 18, txt2)
	dw := float64(x1-x0) / 7
	for i, d := range u.Days {
		bx := x0 + int(float64(i)*dw)
		bh := int(float64(d.Out) / float64(wk) * 40)
		c := green
		if i == len(u.Days)-1 {
			c = amber
		}
		if d.Out > 0 {
			rect(img, bx, 392-max(bh, 2), bx+int(dw)-12, 392, c)
		}
		text(img, float64(bx), 398, strings.ToUpper(d.Day.Format("Mon")), 16, txt3)
		if d.Out > 0 {
			text(img, float64(bx), 392-max(bh, 2)-20, tok(d.Out), 16, txt2)
		}
	}
	vrule(img, ccX2-30)

	// running sessions, then today's projects
	x := ccX2
	head(img, x, pad-4, "sessions now", W-pad-x)
	c.busy = c.busy[:0]
	y := 74
	if len(u.Sessions) == 0 {
		text(img, float64(x), y, "NONE RUNNING", 22, txt3)
		y += 44
	}
	for i, s := range u.Sessions {
		if i == 4 {
			break
		}
		state, sc := "IDLE", txt3
		if s.Busy {
			state, sc = "WORKING", amber
			c.busy = append(c.busy, [2]float64{float64(x + 8), float64(y + 11)})
		}
		disc(img, float64(x+8), float64(y+11), 6, sc)
		if !s.Since.IsZero() {
			state += " " + agoShort(now.Sub(s.Since))
		}
		sw := measure(state, 18)
		text(img, float64(W-pad)-sw, y+2, state, 18, sc)
		text(img, float64(x+28), y, fit(strings.ToUpper(s.Name), 22, float64(W-pad-x-40)-sw), 22, txt)
		// how full the conversation is (scaled to 200K, or 1M past that)
		span := int64(200_000)
		if s.Context > span {
			span = 1_000_000
		}
		bx0, bx1 := x+28, x+260
		bar(img, bx0, y+34, bx1, y+40, float64(s.Context)/float64(span), ramp(float64(s.Context)/float64(span)*1.2-0.2))
		text(img, float64(bx1+14), y+28, fmt.Sprintf("%s CTX   %s OUT TODAY", tok(s.Context), tok(s.Today.Out)), 16, txt2)
		y += 60
	}
	if y < 330 && len(u.Projects) > 0 {
		head(img, x, y+4, "projects today", W-pad-x)
		y += 40
		for _, p := range u.Projects {
			if y > 410 {
				break
			}
			v := tok(p.Out)
			text(img, float64(x), y, fit(strings.ToUpper(p.Name), 18, float64(W-pad-x-100)), 18, txt2)
			text(img, float64(W-pad)-measure(v, 18), y, v, 18, violet)
			y += 26
		}
	}
}

func agoShort(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "NOW"
	case d < time.Hour:
		return fmt.Sprintf("%dM", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dH", int(d.Hours()))
	}
	return fmt.Sprintf("%dD", int(d.Hours()/24))
}
