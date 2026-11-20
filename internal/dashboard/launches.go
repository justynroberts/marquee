package dashboard

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// Launches counts down to the next rocket launch anywhere, live to the second, with its rocket,
// mission and pad, and lists the ones after it. From The Space Devs' Launch Library.
type Launches struct {
	fetch func() ([]feeds.Launch, error)
	poll  poller[[]feeds.Launch]
	cache frameCache
}

func NewLaunches() *Launches {
	return &Launches{fetch: feeds.FetchLaunches, poll: poller[[]feeds.Launch]{every: 30 * time.Minute}}
}

func (l *Launches) Frame(t, dt float64, now time.Time) *image.RGBA {
	ls, got, ok, err := l.poll.get("", func(string) ([]feeds.Launch, error) { return l.fetch() })
	if !ok {
		return waitMessage(&l.cache, "LAUNCHES", err)
	}
	// drop launches more than an hour gone, which the next fetch will too
	for len(ls) > 1 && now.Sub(ls[0].Net) > time.Hour {
		ls = ls[1:]
	}
	if len(ls) == 0 {
		return waitMessage(&l.cache, "LAUNCHES", fmt.Errorf("no launches scheduled"))
	}
	return l.cache.get(fmt.Sprint(got.UnixNano(), now.Unix()), func(img *image.RGBA) { lnRender(img, ls, now) })
}

// lnStatus colours a launch status: Go green, TBC amber, TBD grey, Hold red.
func lnStatus(s string) color.RGBA {
	switch strings.ToUpper(s) {
	case "GO", "SUCCESS":
		return green
	case "TBC":
		return amber
	case "HOLD", "FAILURE", "PARTIAL FAILURE":
		return hot
	case "IN FLIGHT":
		return cyan
	}
	return txt3
}

// lnCountdown is T- (or T+ once it has gone) as days and h:mm:ss.
func lnCountdown(d time.Duration) string {
	sign := "T-"
	if d < 0 {
		sign, d = "T+", -d
	}
	s := int(d.Seconds())
	days := s / 86400
	if days > 0 {
		return fmt.Sprintf("%s %dD %02d:%02d:%02d", sign, days, s/3600%24, s/60%60, s%60)
	}
	return fmt.Sprintf("%s %02d:%02d:%02d", sign, s/3600, s/60%60, s%60)
}

// lnShort is a short "in" for the list: 3D 4H, 5H 12M, 40M.
func lnShort(d time.Duration) string {
	if d < 0 {
		return "NOW"
	}
	m := int(d.Minutes())
	switch {
	case m >= 1440:
		return fmt.Sprintf("%dD %dH", m/1440, m/60%24)
	case m >= 60:
		return fmt.Sprintf("%dH %02dM", m/60, m%60)
	}
	return fmt.Sprintf("%dM", m)
}

// lnWhen is the launch time in local time, as precise as it is known.
func lnWhen(l feeds.Launch, loc *time.Location) string {
	t := l.Net.In(loc)
	switch l.Precision {
	case "Day", "Week":
		return strings.ToUpper(t.Format("Mon 2 Jan"))
	case "Month", "Quarter", "Half", "Year", "Fiscal Year":
		return strings.ToUpper(t.Format("Jan 2006"))
	}
	return strings.ToUpper(t.Format("Mon 2 Jan 15:04"))
}

const lnX1 = 1010

func lnRender(img *image.RGBA, ls []feeds.Launch, now time.Time) {
	n := ls[0]
	head(img, pad, pad-4, "next launch", lnX1-pad-40)
	st := strings.ToUpper(n.Status)
	sc := lnStatus(n.Status)
	text(img, float64(lnX1-70)-measure(st, 22), pad-4, st, 22, sc)
	cd := lnCountdown(n.Net.Sub(now))
	c := amber
	if now.After(n.Net) {
		c = green
	}
	text(img, pad, 72, cd, fitPx(cd, 92, lnX1-pad-70, 100), c)
	y := 190
	if n.Precision != "" && n.Precision != "Second" && n.Precision != "Minute" {
		text(img, pad, y, "TIME TO THE "+strings.ToUpper(n.Precision)+" ONLY", 18, txt3)
	}
	y += 28
	text(img, pad, y, fit(strings.ToUpper(n.Rocket), 34, float64(lnX1-pad-70)), 34, txt)
	y += 48
	mis := n.Mission
	if n.Orbit != "" && n.Orbit != "N/A" {
		mis += "  ·  " + n.Orbit
	}
	text(img, pad, y, fit(strings.ToUpper(mis), 26, float64(lnX1-pad-70)), 26, cyan)
	y += 40
	text(img, pad, y, fit(strings.ToUpper(n.Provider+"  ·  "+lnWhen(n, now.Location())), 22, float64(lnX1-pad-70)), 22, txt2)
	y += 34
	where := n.Location
	if where == "" {
		where = n.Pad
	}
	text(img, pad, y, fit(strings.ToUpper(where), 20, float64(lnX1-pad-70)), 20, txt2)
	if n.Webcast {
		text(img, pad, y+34, "WEBCAST LIVE", 20, hot)
	}
	vrule(img, lnX1-30)

	// the ones after
	x := lnX1 + 10
	head(img, x, pad-4, "coming up", W-pad-x)
	ry := 84
	for _, l := range ls[1:] {
		if ry > H-pad-40 {
			break
		}
		text(img, float64(x), ry, lnShort(l.Net.Sub(now)), 22, amber)
		text(img, float64(x+150), ry+2, fit(strings.ToUpper(l.Status), 18, 60), 18, lnStatus(l.Status))
		name := strings.ToUpper(l.Rocket + "  " + l.Mission)
		text(img, float64(x+230), ry, fit(name, 22, float64(W-pad-x-230)), 22, txt)
		text(img, float64(x+230), ry+28, fit(strings.ToUpper(lnWhen(l, now.Location())+"  ·  "+l.Provider+"  ·  "+l.Country), 16, float64(W-pad-x-230)), 16, txt3)
		ry += 58
	}
}
