package dashboard

import (
	"bufio"
	"fmt"
	"image"
	"os"
	"sort"
	"strings"
	"time"
)

// Countdown counts down to the dates in countdown.txt: the next one huge in days (hours,
// minutes and seconds on the day itself), the ones after it listed on the right.
type Countdown struct {
	events []Event
	bad    []string
	cache  frameCache
}

// Event is a countdown target. Yearly events (given without a year) roll over to next year
// once they have passed.
type Event struct {
	Label  string
	At     time.Time // the next occurrence is worked out from this
	Yearly bool
}

// DefaultCountdown is used when there is no countdown.txt.
var DefaultCountdown = []string{"Christmas = 12-25", "New Year = 01-01"}

// ParseCountdown reads "Label = YYYY-MM-DD", "Label = MM-DD" (every year) or either with
// " HH:MM" after it. It returns the lines it could not read.
func ParseCountdown(lines []string) (evs []Event, bad []string) {
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		label, when, ok := strings.Cut(l, "=")
		label, when = strings.TrimSpace(label), strings.TrimSpace(when)
		if !ok || label == "" {
			bad = append(bad, l)
			continue
		}
		var e Event
		var err error
		for _, f := range []string{"2006-01-02 15:04", "2006-01-02", "01-02 15:04", "01-02"} {
			if e.At, err = time.ParseInLocation(f, when, time.Local); err == nil {
				e.Yearly = !strings.HasPrefix(f, "2006")
				break
			}
		}
		if err != nil {
			bad = append(bad, l)
			continue
		}
		e.Label = label
		evs = append(evs, e)
	}
	return evs, bad
}

// LoadCountdown reads path, falling back to the defaults when it is missing or empty.
func LoadCountdown(path string) *Countdown {
	var lines []string
	if f, err := os.Open(path); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			lines = append(lines, sc.Text())
		}
		f.Close()
	}
	evs, bad := ParseCountdown(lines)
	if len(evs) == 0 {
		evs, _ = ParseCountdown(DefaultCountdown)
	}
	return &Countdown{events: evs, bad: bad}
}

// next is the event's next occurrence on or after the start of today.
func (e Event) next(now time.Time) time.Time {
	at := e.At
	if !e.Yearly {
		return at
	}
	at = time.Date(now.Year(), at.Month(), at.Day(), at.Hour(), at.Minute(), 0, 0, now.Location())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if at.Before(today) {
		at = at.AddDate(1, 0, 0)
	}
	return at
}

type upcoming struct {
	label string
	at    time.Time
}

// daysUntil counts calendar days, so tomorrow is 1 whatever the hour.
func daysUntil(now, at time.Time) int {
	return dayNumber(at.In(now.Location())) - dayNumber(now)
}

func (c *Countdown) list(now time.Time) []upcoming {
	var out []upcoming
	for _, e := range c.events {
		at := e.next(now)
		if daysUntil(now, at) < 0 {
			continue // a one-off that has gone
		}
		out = append(out, upcoming{e.Label, at})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].at.Before(out[j].at) })
	return out
}

func (c *Countdown) Frame(t, dt float64, now time.Time) *image.RGBA {
	return c.cache.get(now.Format("2006-01-02 15:04:05"), func(img *image.RGBA) { c.render(img, now) })
}

const cdSplit = 1260

func (c *Countdown) render(img *image.RGBA, now time.Time) {
	ups := c.list(now)
	caps(img, pad, pad-4, "countdown", 600)
	if len(c.bad) > 0 {
		msg := fmt.Sprintf("COUNTDOWN.TXT: CANNOT READ %q", c.bad[0])
		text(img, float64(cdSplit-40)-measure(fit(msg, 18, 700), 18), pad-2, fit(msg, 18, 700), 18, hot)
	}
	if len(ups) == 0 {
		text(img, pad, H/2-20, "NOTHING TO COUNT DOWN TO: ADD DATES TO COUNTDOWN.TXT", 30, ink3)
		return
	}
	u := ups[0]
	days := daysUntil(now, u.at)
	text(img, pad, 84, fit(strings.ToUpper(u.label), 48, cdSplit-pad-80), 48, ink)
	text(img, pad, 146, strings.ToUpper(u.at.Format("Monday 2 January 2006")), 24, ink3)
	var big, unit string
	col := ink
	switch {
	case days == 0 && now.Before(u.at):
		d := u.at.Sub(now).Round(time.Second)
		big, unit, col = fmt.Sprintf("%d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60), "TO GO", hot
	case days == 0:
		big, unit, col = "TODAY", "", hot
	case days == 1:
		big, unit = "1", "DAY"
	default:
		big, unit = fmt.Sprint(days), "DAYS"
	}
	px := fitPx(big, 230, cdSplit-pad-320, 240)
	b := inkBox(big, px)
	by := H - pad - b.Max.Y
	adv := text(img, pad, by, big, px, col)
	if unit != "" {
		ub := inkBox(unit, 54)
		text(img, float64(pad)+adv+28, by+b.Max.Y-ub.Max.Y, unit, 54, ink2)
	}
	if days > 0 {
		weeks := fmt.Sprintf("%d WEEKS %d DAYS", days/7, days%7)
		if days < 7 {
			weeks = fmt.Sprintf("%d HOURS", int(u.at.Sub(now).Hours()))
		}
		text(img, float64(cdSplit-40)-measure(weeks, 24), 150, weeks, 24, ink3)
	}
	vline(img, cdSplit)

	x := float64(cdSplit + 40)
	caps(img, cdSplit+40, pad-4, "after that", W-pad-cdSplit-40)
	for i, u := range ups[1:] {
		if i >= 6 {
			break
		}
		y := 84 + i*58
		d := daysUntil(now, u.at)
		n := fmt.Sprintf("%d", d)
		text(img, x, y, fit(strings.ToUpper(u.label), 26, float64(W-pad)-x-140), 26, ink)
		text(img, x, y+30, strings.ToUpper(u.at.Format("Mon 2 Jan 2006")), 16, ink3)
		text(img, float64(W-pad)-measure(n, 34), y-4, n, 34, ink2)
	}
}
