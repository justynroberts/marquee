package scenes

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/justynroberts/marquee/internal/led"
)

func init() {
	Register("clock", "scene", "A big clock with the date stacked left and a seconds sweep", func() Scene { return &clock{} })
}

// clock: time on the right, day / date / year+week stacked on the left, seconds bar along the bottom.
type clock struct {
	text string
	mask *led.Mask
}

func (c *clock) Draw(g *led.Grid, t, dt float64) {
	now := time.Now()
	text := now.Format("15:04")
	if text != c.text {
		c.text, c.mask = text, led.Text(text, "sans", 54)
	}
	x0 := cols - c.mask.W - 10
	g.Stamp(c.mask, x0, 0, ink)
	sec := float64(now.Second()) + float64(now.Nanosecond())/1e9
	secondsBar(g, sec/60, alert)
	_, week := now.ISOWeek()
	lines := []struct {
		s         string
		top, size int
		c         led.RGB
	}{
		{strings.ToUpper(now.Format("Mon")), 8, 16, alert},
		{strings.ToUpper(now.Format("02 Jan")), 30, 11, led.RGB{R: 190, G: 186, B: 175}},
		{fmt.Sprintf("%d W%02d", now.Year(), week), 46, 9, led.RGB{R: 120, G: 118, B: 110}},
	}
	for _, l := range lines {
		label(g, l.s, "mono", l.size, 6, l.top, x0-12, l.c)
	}
}

// secondsBar draws a two-row progress bar along the bottom: frac of the width in c, the rest dim.
func secondsBar(g *led.Grid, frac float64, c led.RGB) {
	bar := int(float64(cols) * math.Max(0, math.Min(1, frac)))
	g.Fill(0, rows-3, bar, rows-1, c)
	g.Fill(bar, rows-3, cols, rows-1, track)
}

// Countdown is the engine's timer overlay: a big countdown with a draining bar that turns red
// in the last 10 seconds, then flashes TIME'S UP.
type Countdown struct {
	total float64
	end   time.Time
	label string
	cache map[string]*led.Mask
}

// NewCountdown starts a countdown of seconds with an optional label.
func NewCountdown(seconds float64, label string) *Countdown {
	return &Countdown{
		total: math.Max(1, seconds),
		end:   time.Now().Add(time.Duration(seconds * float64(time.Second))),
		label: strings.ToUpper(label),
		cache: map[string]*led.Mask{},
	}
}

// Remaining is seconds left (negative after zero).
func (c *Countdown) Remaining() float64 { return time.Until(c.end).Seconds() }

func (c *Countdown) mask(s string, size int) *led.Mask {
	k := fmt.Sprintf("%d:%s", size, s)
	if m, ok := c.cache[k]; ok {
		return m
	}
	if len(c.cache) > 8 {
		c.cache = map[string]*led.Mask{}
	}
	m := led.Text(s, "sans", size)
	c.cache[k] = m
	return m
}

// Draw implements Scene.
func (c *Countdown) Draw(g *led.Grid, t, dt float64) {
	left := c.Remaining()
	if left <= 0 {
		if int(t*4)%2 == 0 {
			m := c.mask("TIME'S UP", 50)
			g.Stamp(m, (cols-m.W)/2, 0, alert)
		}
		return
	}
	secs := int(math.Ceil(left))
	text := fmt.Sprintf("%02d:%02d", secs/60, secs%60)
	if secs >= 3600 {
		text = fmt.Sprintf("%d:%02d:%02d", secs/3600, secs%3600/60, secs%60)
	}
	m := c.mask(text, 54)
	x0 := cols - m.W - 10
	urgent := left < 10
	col := ink
	if urgent {
		col = alert
	}
	g.Stamp(m, x0, 0, col)
	if c.label != "" {
		label(g, c.label, "mono", 13, 6, 12, x0-12, amber)
	}
	barCol := green
	if urgent {
		barCol = alert
	}
	secondsBar(g, left/c.total, barCol)
}
