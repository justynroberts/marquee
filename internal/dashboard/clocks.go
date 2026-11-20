package dashboard

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"os"
	"strings"
	"time"
	_ "time/tzdata" // world clock zones work even without a system zoneinfo
)

// ---------------------------------------------------------------- shared

// inkBox is the txt bounds of s at px relative to text()'s (x, y).
func inkBox(s string, px int) image.Rectangle {
	if f := faceFor(s, px); f != nil {
		return f.BBox(s)
	}
	return image.Rectangle{}
}

// fitPx is the largest size up to max whose txt for s fits w x h.
func fitPx(s string, max, w, h int) int {
	px := max
	for px > 8 {
		b := inkBox(s, px)
		if b.Dx() <= w && b.Dy() <= h {
			break
		}
		px -= 2
	}
	return px
}

// frameCache keeps a rendered base image per key (the time as shown) and hands out a fresh copy
// each frame, because the engine post-processes frames in place.
type frameCache struct {
	key  string
	base *image.RGBA
	out  *image.RGBA
}

func (c *frameCache) get(key string, render func(*image.RGBA)) *image.RGBA {
	if c.base == nil {
		c.base = image.NewRGBA(image.Rect(0, 0, W, H))
		c.out = image.NewRGBA(image.Rect(0, 0, W, H))
	}
	if key != c.key {
		fill(c.base, bg)
		render(c.base)
		c.key = key
	}
	copy(c.out.Pix, c.base.Pix)
	return c.out
}

// ---------------------------------------------------------------- big clock

// BigClock is a full-resolution clock: the date stacked on the left, hours and minutes as large
// as the panel allows, seconds beside them and a sweep along the bottom.
type BigClock struct {
	cache  frameCache
	timePx int
	secPx  int
}

const bcLeft = 470 // width of the date column

func NewBigClock() *BigClock {
	c := &BigClock{}
	c.timePx = fitPx("00:00", 440, W-bcLeft-pad-250, H-2*pad-24)
	c.secPx = c.timePx * 3 / 10
	return c
}

func (c *BigClock) Frame(t, dt float64, now time.Time) *image.RGBA {
	img := c.cache.get(now.Format("15:04:05"), func(img *image.RGBA) { c.render(img, now) })
	sec := float64(now.Second()) + float64(now.Nanosecond())/1e9
	y := H - 10
	rect(img, 0, y, W-1, H-1, line)
	rect(img, 0, y, int(float64(W-1)*sec/60), H-1, amber)
	return img
}

func (c *BigClock) render(img *image.RGBA, now time.Time) {
	// date column
	_, week := now.ISOWeek()
	name, off := now.Zone()
	text(img, pad, 44, strings.ToUpper(now.Format("Monday")), 44, cyan)
	text(img, pad, 104, strings.ToUpper(now.Format("2 Jan")), 96, txt)
	text(img, pad, 236, fmt.Sprintf("%d  W%02d", now.Year(), week), 40, amber)
	text(img, pad, 300, fmt.Sprintf("DAY %d", now.YearDay()), 28, txt3)
	text(img, pad, 340, fmt.Sprintf("%s  UTC%s", name, utcOffset(off)), 28, txt3)
	vrule(img, bcLeft-40)

	// time, vertically centred on its txt
	hm := now.Format("15:04")
	b := inkBox(hm, c.timePx)
	y := (H-10-b.Dy())/2 - b.Min.Y
	adv := text(img, bcLeft, y, hm, c.timePx, txt)
	// seconds share the digits' baseline
	sb := inkBox("00", c.secPx)
	sy := y + b.Max.Y - sb.Max.Y
	text(img, float64(bcLeft)+adv+24, sy, now.Format(":05"), c.secPx, amber)
}

func utcOffset(sec int) string {
	sign := "+"
	if sec < 0 {
		sign, sec = "−", -sec
	}
	if sec%3600 == 0 {
		return fmt.Sprintf("%s%d", sign, sec/3600)
	}
	return fmt.Sprintf("%s%d:%02d", sign, sec/3600, sec%3600/60)
}

// ---------------------------------------------------------------- world clock

// Zone is one world clock column.
type Zone struct {
	Label string
	Loc   *time.Location
}

// DefaultZones are shown when no world-clock.txt exists.
var DefaultZones = []string{
	"San Francisco = America/Los_Angeles",
	"New York = America/New_York",
	"London = Europe/London",
	"Tokyo = Asia/Tokyo",
	"Sydney = Australia/Sydney",
}

// maxZones keeps every column wide enough for a readable time.
const maxZones = 8

// ParseZones reads "Label = Area/City" lines (or a bare zone name, labelled by its city);
// blank lines and # comments are skipped, unknown zones are reported.
func ParseZones(lines []string) ([]Zone, []string) {
	var out []Zone
	var bad []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		label, name, ok := strings.Cut(l, "=")
		if !ok {
			name = l
			label = l[strings.LastIndex(l, "/")+1:]
			label = strings.ReplaceAll(label, "_", " ")
		}
		label, name = strings.TrimSpace(label), strings.TrimSpace(name)
		loc, err := time.LoadLocation(name)
		if err != nil || name == "" {
			bad = append(bad, name)
			continue
		}
		if len(out) < maxZones {
			out = append(out, Zone{Label: label, Loc: loc})
		}
	}
	return out, bad
}

// LoadZones reads path, falling back to DefaultZones when it is missing or lists nothing usable.
func LoadZones(path string) []Zone {
	var lines []string
	if f, err := os.Open(path); err == nil {
		s := bufio.NewScanner(f)
		for s.Scan() {
			lines = append(lines, s.Text())
		}
		f.Close()
	}
	if z, _ := ParseZones(lines); len(z) > 0 {
		return z
	}
	z, _ := ParseZones(DefaultZones)
	return z
}

// WorldClock shows one column per zone: city, time, day and date relative to here, and a
// 24-hour strip with daylight hours lit and a marker at the current time. Columns where it is
// night are dimmed; the column for this Mac's own offset is marked HERE.
type WorldClock struct {
	zones  []Zone
	cache  frameCache
	timePx int
}

func NewWorldClock(zones []Zone) *WorldClock {
	if len(zones) == 0 {
		zones, _ = ParseZones(DefaultZones)
	}
	n := len(zones)
	colW := (W - 2*pad) / n
	return &WorldClock{zones: zones, timePx: fitPx("00:00", 200, colW-40, 170)}
}

func (c *WorldClock) Frame(t, dt float64, now time.Time) *image.RGBA {
	return c.cache.get(now.Format("15:04"), func(img *image.RGBA) { c.render(img, now) })
}

func (c *WorldClock) render(img *image.RGBA, now time.Time) {
	n := len(c.zones)
	colW := (W - 2*pad) / n
	_, here := now.Zone()
	hereDay := dayNumber(now)
	labelPx := 30
	if n > 6 {
		labelPx = 24
	}
	tb := inkBox("00:00", c.timePx)
	timeTop := 104
	for i, z := range c.zones {
		x := pad + i*colW
		if i > 0 {
			wideLine(img, x-20, pad, x-20, H-pad, line, 2)
		}
		lt := now.In(z.Loc)
		_, off := lt.Zone()
		h := float64(lt.Hour()) + float64(lt.Minute())/60
		day := h >= 7 && h < 19
		w := float64(colW - 40)

		tag := strings.ToUpper(z.Label)
		col := cyan
		if off == here {
			col = amber
		}
		text(img, float64(x), pad, fit(tag, labelPx, w), labelPx, col)

		tc := txt
		if !day {
			tc = violet // night there
		}
		text(img, float64(x), timeTop-tb.Min.Y, lt.Format("15:04"), c.timePx, tc)

		y := timeTop + tb.Dy() + 30
		text(img, float64(x), y, strings.ToUpper(lt.Format("Mon 2 Jan")), 28, txt2)
		rel := "HERE"
		if off != here {
			rel = utcOffsetHours(off - here)
			switch dayNumber(lt) - hereDay {
			case 1:
				rel += "  TOMORROW"
			case -1:
				rel += "  YESTERDAY"
			}
		}
		text(img, float64(x), y+42, fit(rel, 24, w), 24, txt3)

		// 24-hour strip
		sy0, sy1 := H-pad-26, H-pad
		sw := colW - 40
		rect(img, x, sy0, x+sw, sy1, line)
		rect(img, x+sw*7/24, sy0, x+sw*19/24, sy1, color.RGBA{120, 96, 40, 255}) // daylight
		for hr := 0; hr < 24; hr += 6 {
			text(img, float64(x+sw*hr/24), sy0-34, fmt.Sprintf("%02d", hr), 20, txt3)
		}
		mx := x + int(float64(sw)*h/24)
		rect(img, mx-2, sy0-8, mx+2, sy1+8, amber)
	}
}

// dayNumber orders calendar dates in their own zone.
func dayNumber(t time.Time) int {
	y, m, d := t.Date()
	return int(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400)
}

func utcOffsetHours(sec int) string {
	if sec == 0 {
		return "±0H"
	}
	s := utcOffset(sec)
	return s + "H"
}
