package dashboard

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"
)

// Progress is time as progress bars: how far through this hour, today, this week (ISO, from
// Monday), this month, quarter, year, decade and century we are, the percentages ticking live,
// with the year's share large and a few counts beside it (day of the year, ISO week, days left,
// working days left this month).
type Progress struct {
	cache frameCache
}

func NewProgress() *Progress { return &Progress{} }

type pgSpan struct {
	label    string
	from, to time.Time
	places   int // decimals
	colour   color.RGBA
}

// pgSpans are the periods now falls in, shortest first.
func pgSpans(now time.Time) []pgSpan {
	loc := now.Location()
	y, m, d := now.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, loc)
	hour := time.Date(y, m, d, now.Hour(), 0, 0, 0, loc)
	week := pgWeekStart(now)
	month := time.Date(y, m, 1, 0, 0, 0, 0, loc)
	q := time.Date(y, (m-1)/3*3+1, 1, 0, 0, 0, 0, loc)
	year := time.Date(y, 1, 1, 0, 0, 0, 0, loc)
	dec := time.Date(y/10*10, 1, 1, 0, 0, 0, 0, loc)
	cy := (y-1)/100*100 + 1 // centuries run 2001-2100
	cent := time.Date(cy, 1, 1, 0, 0, 0, 0, loc)
	return []pgSpan{
		{"HOUR", hour, hour.Add(time.Hour), 2, violet},
		{"TODAY", day, day.AddDate(0, 0, 1), 3, cyan},
		{"WEEK " + fmt.Sprint(pgISOWeek(now)), week, week.AddDate(0, 0, 7), 4, green},
		{strings.ToUpper(now.Format("January")), month, month.AddDate(0, 1, 0), 4, cyan},
		{fmt.Sprintf("Q%d", (int(m)-1)/3+1), q, q.AddDate(0, 3, 0), 5, violet},
		{fmt.Sprint(y), year, year.AddDate(1, 0, 0), 5, amber},
		{fmt.Sprintf("THE %dS", y/10*10), dec, dec.AddDate(10, 0, 0), 6, green},
		{"CENTURY " + pgOrdinal(cy/100+1), cent, cent.AddDate(100, 0, 0), 7, violet},
	}
}

// pgWeekStart is midnight on the Monday of now's week.
func pgWeekStart(now time.Time) time.Time {
	y, m, d := now.Date()
	wd := (int(now.Weekday()) + 6) % 7 // Monday 0
	return time.Date(y, m, d-wd, 0, 0, 0, 0, now.Location())
}

func pgISOWeek(now time.Time) int { _, w := now.ISOWeek(); return w }

func pgFrac(now, from, to time.Time) float64 {
	f := float64(now.Sub(from)) / float64(to.Sub(from))
	return max(0, min(f, 1))
}

func pgOrdinal(n int) string {
	suf := "TH"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suf = "ST"
		case 2:
			suf = "ND"
		case 3:
			suf = "RD"
		}
	}
	return fmt.Sprint(n, suf)
}

// pgWorkdaysLeft counts Monday to Friday from today (included) to the end of the month. Bank
// holidays are not taken off.
func pgWorkdaysLeft(now time.Time) int {
	y, m, d := now.Date()
	n := 0
	for t := time.Date(y, m, d, 12, 0, 0, 0, now.Location()); t.Month() == m; t = t.AddDate(0, 0, 1) {
		if wd := t.Weekday(); wd != time.Saturday && wd != time.Sunday {
			n++
		}
	}
	return n
}

// pgDaysInYear and the day of the year.
func pgDaysInYear(y int) int {
	return time.Date(y, 12, 31, 12, 0, 0, 0, time.UTC).YearDay()
}

const (
	pgX1    = 600  // bars column
	pgBarX0 = 830  // bar start
	pgBarX1 = 1600 // bar end
	pgRowY  = 34
	pgRowH  = 50
)

func (p *Progress) Frame(t, dt float64, now time.Time) *image.RGBA {
	spans := pgSpans(now)
	img := p.cache.get(now.Format("2006-01-02 15"), func(img *image.RGBA) { p.render(img, spans, now) })
	// what moves: the bars, the percentages and the big year figure
	for i, s := range spans {
		y := pgRowY + i*pgRowH
		f := pgFrac(now, s.from, s.to)
		w := int(float64(pgBarX1-pgBarX0) * f)
		if w > 0 {
			rect(img, pgBarX0, y+8, pgBarX0+w, y+26, s.colour)
		}
		text(img, float64(pgBarX1+28), y+1, fmt.Sprintf("%.*f%%", s.places, 100*f), 26, txt)
	}
	yr := spans[5]
	big := fmt.Sprintf("%.6f%%", 100*pgFrac(now, yr.from, yr.to))
	text(img, pad, 96, big, fitPx(big, 92, pgX1-pad-60, 120), amber)
	return img
}

func (p *Progress) render(img *image.RGBA, spans []pgSpan, now time.Time) {
	y := now.Year()
	head(img, pad, pad-4, fmt.Sprintf("%d is", y), pgX1-pad-40)
	text(img, pad, 210, "OF THE WAY THROUGH", 22, txt2)
	doy, days := now.YearDay(), pgDaysInYear(y)
	rows := []struct {
		label, value string
		c            color.RGBA
	}{
		{"DAY", fmt.Sprintf("%d OF %d", doy, days), txt},
		{"ISO WEEK", fmt.Sprint(pgISOWeek(now)), green},
		{"DAYS LEFT", fmt.Sprint(days - doy), amber},
		{"WORKDAYS LEFT", fmt.Sprintf("%d IN %s", pgWorkdaysLeft(now), strings.ToUpper(now.Format("Jan"))), violet},
	}
	for i, r := range rows {
		ry := 262 + i*40
		text(img, pad, ry, r.label, 22, cyan)
		text(img, float64(pad+250), ry-3, r.value, 28, r.c)
	}
	vrule(img, pgX1-30)
	for i, s := range spans {
		ry := pgRowY + i*pgRowH
		text(img, pgX1, ry+1, fit(s.label, 24, pgBarX0-pgX1-24), 24, cyan)
		rect(img, pgBarX0, ry+8, pgBarX1, ry+26, mix(bg, s.colour, 0.16))
		// quarter ticks along each bar
		for k := 1; k < 4; k++ {
			x := pgBarX0 + (pgBarX1-pgBarX0)*k/4
			rect(img, x, ry+27, x, ry+31, txt3)
		}
	}
}
