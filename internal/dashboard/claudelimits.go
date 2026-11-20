package dashboard

import (
	"fmt"
	"image"
	"math"
	"strings"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// ClaudeLimits shows how much of the Claude plan is left: the five-hour session and the week,
// each as a big "percent left" over a fuel bar, with a pace mark showing how far through the
// window we are (used ahead of the mark means running hot). On the right, the week by app.

type ClaudeLimits struct {
	poll  poller[*feeds.ClaudeLimits]
	fetch func() (*feeds.ClaudeLimits, error)
	cache frameCache
	shown [2]float64 // eased fill, percent used
	want  [2]float64
	warn  [2]bool
}

func NewClaudeLimits() *ClaudeLimits {
	return &ClaudeLimits{poll: poller[*feeds.ClaudeLimits]{every: 3 * time.Minute}, fetch: feeds.FetchClaudeLimits}
}

const (
	lmX0   = 340
	lmX1   = 1430
	lmRow  = 215
	lmBar  = 150 // bar top within a row
	lmBarH = 40
)

func (c *ClaudeLimits) Frame(t, dt float64, now time.Time) *image.RGBA {
	l, got, ok, err := c.poll.get("", func(string) (*feeds.ClaudeLimits, error) { return c.fetch() })
	if !ok {
		return waitMessage(&c.cache, "CLAUDE PLAN LIMITS", err)
	}
	img := c.cache.get(fmt.Sprint(got.UnixNano(), now.Format("15:04")), func(img *image.RGBA) { c.base(img, l, got, now) })
	for i := range c.shown {
		c.shown[i] += (c.want[i] - c.shown[i]) * math.Min(1, dt*2)
		y := 30 + i*lmRow + lmBar
		w := float64(lmX1 - lmX0)
		used := int(w * math.Min(1, c.shown[i]/100))
		col := ramp(c.shown[i]/100*1.25 - 0.25)
		if c.warn[i] {
			col = mix(col, txt, 0.25*(0.5+0.5*math.Sin(t*4)))
		}
		rect(img, lmX0, y, lmX0+used, y+lmBarH, mix(bg, col, 0.9))
		// the remaining fuel glows faintly, brighter at its leading edge
		if used < int(w) {
			rect(img, lmX0+used, y, lmX0+used+4, y+lmBarH, txt)
		}
	}
	ccLogo(img, 150, 170, 92, t*0.15, claudeOrange)
	return img
}

func (c *ClaudeLimits) base(img *image.RGBA, l *feeds.ClaudeLimits, got, now time.Time) {
	// left: the spark and the plan
	plan := l.Plan
	text(img, 150-measure(plan, 44)/2, 300, plan, 44, claudeOrange)
	s := "PLAN LIMITS"
	text(img, 150-measure(s, 20)/2, 360, s, 20, cyan)
	s = "AS OF " + got.Format("15:04")
	text(img, 150-measure(s, 20)/2, 392, s, 20, txt3)

	for i, lim := range []feeds.ClaudeLimit{l.Session, l.Week} {
		y := 30 + i*lmRow
		c.want[i], c.warn[i] = lim.Used, lim.Warning
		title := "CURRENT SESSION  5 HOURS"
		if i == 1 {
			title = "THIS WEEK  ALL MODELS"
		}
		text(img, lmX0, y, title, 22, cyan)
		left := lim.Left()
		col := green
		switch {
		case left <= 10:
			col = hot
		case left <= 30 || lim.Warning:
			col = amber
		}
		n := fmt.Sprintf("%.0f%%", left)
		x := text(img, lmX0, y+30, n, 100, col)
		text(img, lmX0+x+16, y+78, "LEFT", 40, txt2)

		// reset, right-aligned
		if !lim.Resets.IsZero() {
			in := lim.Resets.Sub(now)
			r1 := "RESETS IN " + untilShort(in)
			r2 := strings.ToUpper(lim.Resets.Local().Format("Mon 15:04"))
			if in < 24*time.Hour {
				r2 = lim.Resets.Local().Format("15:04")
			}
			text(img, lmX1-measure(r1, 34), y+44, r1, 34, txt)
			text(img, lmX1-measure(r2, 24), y+92, r2, 24, txt3)
		}

		// the bar's track, quarter ticks and the pace mark
		by := y + lmBar
		rect(img, lmX0, by, lmX1, by+lmBarH, mix(bg, line, 0.8))
		for q := 1; q < 4; q++ {
			qx := lmX0 + (lmX1-lmX0)*q/4
			rect(img, qx-1, by+lmBarH-8, qx+1, by+lmBarH, txt3)
		}
		if p, ok := pace(lim, now); ok {
			px := lmX0 + int(float64(lmX1-lmX0)*p)
			rect(img, px-2, by-10, px+2, by+lmBarH+10, txt)
			msg, mc := "ON PACE", green
			switch d := lim.Used/100 - p; {
			case d > 0.08:
				msg, mc = "AHEAD OF PACE", amber
			case d < -0.08:
				msg, mc = "UNDER PACE", green
			}
			if lim.Used >= 100 {
				msg, mc = "LIMIT REACHED", hot
			}
			mx := float64(px) - measure(msg, 20)/2
			mx = math.Max(lmX0, math.Min(float64(lmX1)-measure(msg, 20), mx))
			text(img, mx, by-34, msg, 20, mc)
		}
	}

	// right: the week by app, then per-model limits
	x0, x1 := 1500, W-pad
	wideLine(img, x0-34, pad, x0-34, H-pad, line, 2)
	text(img, float64(x0), 30, "THE WEEK BY APP", 22, cyan)
	y := 74
	for _, sh := range l.Shares {
		name := fit(strings.ToUpper(sh.Name), 26, float64(x1-x0-90))
		text(img, float64(x0), y, name, 26, txt2)
		p := fmt.Sprintf("%.0f%%", sh.Percent)
		text(img, float64(x1)-measure(p, 26), y, p, 26, amber)
		rect(img, x0, y+38, x1, y+44, mix(bg, line, 0.8))
		rect(img, x0, y+38, x0+int(float64(x1-x0)*sh.Percent/100), y+44, claudeOrange)
		y += 62
	}
	for _, sc := range l.Scoped {
		if y > H-70 {
			break
		}
		y += 6
		name := fit(sc.Name+" WEEK", 26, float64(x1-x0-130))
		text(img, float64(x0), y, name, 26, txt2)
		p := fmt.Sprintf("%.0f%% LEFT", sc.Left())
		text(img, float64(x1)-measure(p, 26), y, p, 26, ramp(sc.Used/100*1.25-0.25))
		y += 50
	}
	if l.Extra && y < H-50 {
		text(img, float64(x0), y+6, "EXTRA USAGE ON", 22, violet)
	}
}

// pace is how far through its window a limit is, 0..1.
func pace(l feeds.ClaudeLimit, now time.Time) (float64, bool) {
	if l.Resets.IsZero() || l.Window <= 0 {
		return 0, false
	}
	p := 1 - l.Resets.Sub(now).Seconds()/l.Window.Seconds()
	return math.Max(0, math.Min(1, p)), true
}

// untilShort is "2D 13H", "3H 08M" or "45M".
func untilShort(d time.Duration) string {
	if d < time.Minute {
		return "MOMENTS"
	}
	m := int(d.Minutes())
	switch {
	case m >= 48*60:
		return fmt.Sprintf("%dD %dH", m/1440, m%1440/60)
	case m >= 60:
		return fmt.Sprintf("%dH %02dM", m/60, m%60)
	}
	return fmt.Sprintf("%dM", m)
}
