package dashboard

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// Calendar is this month as a grid with ISO week numbers, today picked out in txt and bank
// holidays boxed, then next month small and the coming bank holidays. Holidays are the UK's
// from gov.uk for the area in calendar.txt ("none" turns them off).
type Calendar struct {
	area  func() string
	fetch func(string) ([]feeds.Holiday, error)
	poll  poller[[]feeds.Holiday]
	cache frameCache
}

func NewCalendar(area func() string) *Calendar {
	return &Calendar{area: area, fetch: feeds.FetchHolidays, poll: poller[[]feeds.Holiday]{every: 24 * time.Hour}}
}

func (c *Calendar) Frame(t, dt float64, now time.Time) *image.RGBA {
	area := c.area()
	var hs []feeds.Holiday
	var err error
	if area != "none" {
		hs, _, _, err = c.poll.get(area, c.fetch)
	}
	note := ""
	switch {
	case area == "none":
		note = "TURNED OFF IN CALENDAR.TXT"
	case err != nil && hs == nil:
		note = strings.ToUpper(err.Error())
	case hs == nil:
		note = "FETCHING…"
	}
	return c.cache.get(fmt.Sprint(now.Format("2006-01-02"), area, len(hs), note), func(img *image.RGBA) { c.render(img, now, hs, note) })
}

var holidayBox = color.RGBA{82, 60, 22, 255}

func dayKey(t time.Time) string { return t.Format("2006-01-02") }

func (c *Calendar) render(img *image.RGBA, now time.Time, hs []feeds.Holiday, note string) {
	hol := map[string]string{}
	for _, h := range hs {
		hol[dayKey(h.Date)] = h.Title
	}
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	text(img, pad, pad-8, strings.ToUpper(month.Format("January 2006")), 44, txt)
	monthGrid(img, month, now, hol, pad, 96, 150, 56, 34, true)
	vrule(img, 1210)

	x := 1250
	next := month.AddDate(0, 1, 0)
	head(img, x, pad-4, next.Format("January 2006"), W-pad-x)
	monthGrid(img, next, now, hol, x, 72, 84, 32, 20, false)

	y := 300
	head(img, x, y, "bank holidays", W-pad-x)
	if note != "" {
		text(img, float64(x), y+36, fit(note, 18, float64(W-pad-x)), 18, txt3)
		return
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	n := 0
	for _, h := range hs {
		if h.Date.Before(today) || n == 3 {
			continue
		}
		yy := y + 34 + n*36
		days := daysUntil(now, h.Date)
		when := strings.ToUpper(h.Date.Format("Mon 2 Jan"))
		in := fmt.Sprintf("%dD", days)
		if days == 0 {
			in = "TODAY"
		}
		text(img, float64(x), yy, when, 22, txt)
		text(img, float64(x+170), yy, fit(strings.ToUpper(h.Title), 22, float64(W-pad-x-170-90)), 22, amber)
		text(img, float64(W-pad)-measure(in, 22), yy, in, 22, txt2)
		n++
	}
}

// monthGrid draws month's days Monday first at (x, y), each cell cw x ch with numbers at px,
// after a week-number column when weeks is set.
func monthGrid(img *image.RGBA, month, now time.Time, hol map[string]string, x, y, cw, ch, px int, weeks bool) {
	x0 := x
	if weeks {
		x0 += cw / 2
	}
	hdr := px * 2 / 3
	for i, d := range []string{"MON", "TUE", "WED", "THU", "FRI", "SAT", "SUN"} {
		if !weeks {
			d = d[:1]
		}
		c := cyan
		if i >= 5 {
			c = violet
		}
		text(img, float64(x0+i*cw), y, d, hdr, c)
	}
	if weeks {
		text(img, float64(x), y, "WK", hdr, txt3)
	}
	y += hdr + 16
	off := (int(month.Weekday()) + 6) % 7 // Monday = 0
	days := time.Date(month.Year(), month.Month()+1, 0, 0, 0, 0, 0, month.Location()).Day()
	if weeks { // six-week months squeeze up to stay on the panel
		ch = min(ch, (H-pad/2-y)/((off+days+6)/7))
	}
	for d := 1; d <= days; d++ {
		cell := off + d - 1
		col, row := cell%7, cell/7
		cx, cy := x0+col*cw, y+row*ch
		day := time.Date(month.Year(), month.Month(), d, 0, 0, 0, 0, month.Location())
		if weeks && (col == 0 || d == 1) {
			_, wk := day.ISOWeek()
			text(img, float64(x), cy, fmt.Sprintf("%02d", wk), px*2/3, txt3)
		}
		s := fmt.Sprint(d)
		c := txt
		if col >= 5 {
			c = violet // weekends
		}
		b := inkBox("00", px)
		bx0, by0, bx1, by1 := cx-px/3, cy+b.Min.Y-px/4, cx+int(measure("00", px))+px/3, cy+b.Max.Y+px/4
		if _, ok := hol[dayKey(day)]; ok {
			rect(img, bx0, by0, bx1, by1, holidayBox)
			c = amber
		}
		if dayKey(day) == dayKey(now) {
			rect(img, bx0, by0, bx1, by1, amber)
			c = bg
		}
		text(img, float64(cx), cy, s, px, c)
	}
}
