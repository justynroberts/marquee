package dashboard

import (
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// ------------------------------------------------------------------------------------ widgets

// WidgetView is one pushed reading as the widgets screen draws it.
type WidgetView struct {
	Label, Value, Unit, Status, Text string
	Updated                          time.Time
}

// Widgets lays pushed readings out as a board: up to four across in one row, then two rows,
// then three, each with its label, its value large in its status colour, a line of text and
// how old it is. Bad readings pulse.
type Widgets struct {
	get   func() []WidgetView
	cache frameCache
	bad   []image.Rectangle
}

func NewWidgets(get func() []WidgetView) *Widgets { return &Widgets{get: get} }

func statusColour(s string) color.RGBA {
	switch s {
	case "ok":
		return green
	case "warn":
		return amber
	case "bad":
		return hot
	}
	return txt
}

func (w *Widgets) Frame(t, dt float64, now time.Time) *image.RGBA {
	ws := w.get()
	h := fnv.New64a()
	for _, v := range ws {
		fmt.Fprint(h, v.Label, v.Value, v.Unit, v.Status, v.Text, v.Updated.UnixNano(), "|")
	}
	img := w.cache.get(fmt.Sprint(h.Sum64(), now.Format("15:04")), func(img *image.RGBA) { w.render(img, ws, now) })
	k := 0.5 + 0.5*math.Sin(t*4)
	for _, r := range w.bad {
		rect(img, r.Min.X, r.Min.Y, r.Max.X, r.Max.Y, mix(hot, bg, 0.6*k))
	}
	return img
}

func (w *Widgets) render(img *image.RGBA, ws []WidgetView, now time.Time) {
	w.bad = w.bad[:0]
	if len(ws) == 0 {
		head(img, pad, pad-4, "widgets", W-2*pad)
		text(img, pad, 80, "NOTHING POSTED YET", 64, txt3)
		text(img, pad, 190, `curl -d '{"id":"build","label":"Build","value":"PASS","status":"ok"}' localhost:7781/v1/widgets`, 20, txt2)
		text(img, pad, 236, "FROM ANYWHERE: SET A RELAY TOPIC IN SETTINGS AND POST THE SAME JSON TO ntfy.sh/<topic>", 20, txt2)
		text(img, pad, 282, "STATUS ok · warn · bad · info     TTL IN SECONDS DROPS IT     FLASH:TRUE ALSO FLASHES IT", 20, txt3)
		return
	}
	rows := 1
	switch {
	case len(ws) > 8:
		rows = 3
	case len(ws) > 4:
		rows = 2
	}
	cols := (len(ws) + rows - 1) / rows
	y0, y1 := 18, H-14
	tw := float64(W-2*pad+24) / float64(cols)
	th := float64(y1-y0) / float64(rows)
	lpx, vmax, tpx := [4]int{0, 30, 24, 20}[rows], [4]int{0, 128, 76, 48}[rows], [4]int{0, 26, 20, 18}[rows]
	for i, v := range ws {
		r, c := i/cols, i%cols
		x := pad - 12 + int(float64(c)*tw)
		y := y0 + int(float64(r)*th)
		x1, yb := x+int(tw)-24, y+int(th)-12
		sc := statusColour(v.Status)
		bar := image.Rect(x, y+4, x+6, yb)
		rect(img, bar.Min.X, bar.Min.Y, bar.Max.X, bar.Max.Y, sc)
		if v.Status == "bad" {
			w.bad = append(w.bad, bar)
		}
		cx := x + 22
		width := float64(x1 - cx)
		text(img, float64(cx), y+4, fit(strings.ToUpper(v.Label), lpx, width), lpx, cyan)
		vy := y + 4 + lpx + 10
		if v.Value != "" {
			px := vmax
			for px > 20 && measure(v.Value, px)+measure(" "+v.Unit, px/2) > width {
				px -= 4
			}
			adv := text(img, float64(cx), vy, v.Value, px, sc)
			if v.Unit != "" {
				text(img, float64(cx)+adv+8, vy+px/2, v.Unit, px/2, txt2)
			}
			vy += px + 14
		}
		if v.Text != "" && vy+tpx <= yb {
			lines := 1
			if v.Value == "" {
				lines = max(1, (yb-vy-20)/(tpx+8))
			}
			for k, l := range wrap(v.Text, tpx, width, lines) {
				text(img, float64(cx), vy+k*(tpx+8), l, tpx, txt2)
			}
		}
		if rows < 3 {
			age := agoShort(now.Sub(v.Updated))
			if age != "NOW" {
				age += " AGO"
			}
			text(img, float64(x1)-measure(age, 16), yb-20, age, 16, txt3)
		}
		if c > 0 {
			rect(img, x-12, y+4, x-12, yb, line)
		}
	}
}

// ------------------------------------------------------------------------------------ live

// ClaudeLive shows what Claude Code is doing this moment, from its hooks: each active
// session's current tool large, with what it is working on, and a feed of the latest calls.
type ClaudeLive struct {
	log *feeds.ActivityLog
}

func NewClaudeLive() *ClaudeLive { return &ClaudeLive{log: feeds.Activity} }

var lvOut = image.NewRGBA(image.Rect(0, 0, W, H))

func (c *ClaudeLive) Frame(t, dt float64, now time.Time) *image.RGBA {
	img := lvOut
	fill(img, bg)
	type sess struct {
		id string
		n  feeds.ClaudeNow
	}
	var ss []sess
	for id, n := range c.log.Now() {
		if now.Sub(n.Since) < 2*time.Hour {
			ss = append(ss, sess{id, n})
		}
	}
	sort.Slice(ss, func(i, j int) bool {
		ai, aj := ss[i].n.State != "idle", ss[j].n.State != "idle"
		if ai != aj {
			return ai
		}
		return ss[i].n.Since.After(ss[j].n.Since)
	})
	const x1 = 1180
	ccLogo(img, pad+22, pad+14, 20, t*0.4, claudeOrange)
	text(img, pad+54, pad-4, "CLAUDE  NOW", 26, claudeOrange)
	if len(ss) == 0 {
		text(img, pad, 150, "WAITING FOR CLAUDE CODE", 56, txt3)
		text(img, pad, 240, "THE HOOK REPORTS EACH TOOL CALL HERE: SEE THE GUIDE, 'LIVE CLAUDE ACTIVITY'", 20, txt2)
	}
	rows := min(len(ss), 3)
	rh := 0
	if rows > 0 {
		rh = (H - 86) / rows
	}
	for i := 0; i < rows; i++ {
		n := ss[i].n
		y := 84 + i*rh
		if i > 0 {
			rect(img, pad, y-8, x1-40, y-8, line)
		}
		name := strings.ToUpper(n.Project)
		if name == "" {
			name = "SESSION"
		}
		text(img, pad, y, fit(name, 24, 520), 24, claudeOrange)
		age := agoShort(now.Sub(n.Since))
		tools := "TOOLS"
		if n.Calls == 1 {
			tools = "TOOL"
		}
		info := fmt.Sprintf("%d %s THIS TURN  ·  %s", n.Calls, tools, age)
		text(img, float64(x1-40)-measure(info, 18), y+4, info, 18, txt2)
		big := max(36, min(84, rh-70))
		switch n.State {
		case "tool":
			tc := toolColour(n.Tool)
			k := 0.5 + 0.5*math.Sin(t*5)
			disc(img, pad+14, float64(y+40+big/2), 9+3*k, tc)
			adv := text(img, pad+40, y+34, strings.ToUpper(n.Tool), big, tc)
			text(img, pad+40+adv+24, y+34+big/2-10, fit(n.Detail, 26, float64(x1-80)-(pad+40+adv+24)), 26, txt)
		case "thinking":
			dots := strings.Repeat(".", int(t*3)%4)
			text(img, pad+40, y+34, "THINKING"+dots, big, violet)
		default:
			text(img, pad+40, y+34, "IDLE", big, txt3)
		}
	}
	vrule(img, x1)

	// the feed, newest at the top
	x := float64(x1 + 30)
	head(img, int(x), pad-4, "latest calls", W-pad-int(x))
	evs := c.log.Recent(11)
	for i := len(evs) - 1; i >= 0; i-- {
		ev := evs[i]
		y := 70 + (len(evs)-1-i)*35
		col := txt2
		if i == len(evs)-1 {
			col = txt
		}
		text(img, x, y, ev.At.Format("15:04:05"), 18, txt3)
		label, lc := strings.ToUpper(ev.Tool), toolColour(ev.Tool)
		switch ev.Kind {
		case "prompt":
			label, lc = "PROMPT", violet
		case "stop":
			label, lc = "DONE", green
		}
		adv := text(img, x+108, y, label, 18, lc)
		rest := strings.ToUpper(ev.Project)
		if ev.Detail != "" {
			rest = ev.Detail
		}
		text(img, x+108+adv+14, y, fit(rest, 18, float64(W-pad)-(x+108+adv+14)), 18, col)
	}
	return img
}
