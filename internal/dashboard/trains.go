package dashboard

import (
	"errors"
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// Trains is a National Rail station board: departures on the left, arrivals on the
// right, each with the timetabled time, where from or to, the platform and how it is running:
// green on time, amber late, vermilion cancelled. Data is National Rail's (Darwin), through the
// Rail Data Marketplace.
type Trains struct {
	station func() string
	token   func() string
	fetch   func(token, station string, window time.Duration) (*feeds.Board, error)
	poll    poller[*feeds.Board]
	cache   frameCache
}

func NewTrains(station, token func() string) *Trains {
	return &Trains{station: station, token: token, fetch: feeds.FetchBoard, poll: poller[*feeds.Board]{every: time.Minute}}
}

func (tr *Trains) Frame(t, dt float64, now time.Time) *image.RGBA {
	st := strings.ToUpper(tr.station())
	b, got, ok, err := tr.poll.get(st, func(st string) (*feeds.Board, error) { return tr.fetch(tr.token(), st, 2*time.Hour) })
	if !ok {
		if errors.Is(err, feeds.ErrNoToken) {
			return tr.cache.get("token", tokenHelp)
		}
		return waitMessage(&tr.cache, "TRAIN TIMES", err)
	}
	return tr.cache.get(fmt.Sprint(st, got.UnixNano(), now.Format("15:04")), func(img *image.RGBA) { tr.render(img, b, now) })
}

func tokenHelp(img *image.RGBA) {
	head(img, pad, pad-4, "train times", 600)
	text(img, pad, 100, "TRAIN TIMES NEED A FREE NATIONAL RAIL KEY", 40, txt)
	text(img, pad, 172, "1   SIGN UP FREE AT RAILDATA.ORG.UK", 28, cyan)
	text(img, pad, 218, "2   SUBSCRIBE TO  LIVE ARRIVAL AND DEPARTURE BOARDS  (FREE)", 28, cyan)
	text(img, pad, 264, "3   MENU BAR  >  SETTINGS…  >  TRAIN TIMES KEY, AND PASTE ITS CONSUMER KEY", 28, cyan)
	text(img, pad, 330, "LONG EATON (LGE) IS SHOWN; CHANGE THE STATION IN SETTINGS", 22, txt2)
	text(img, pad, 370, "THE KEY STAYS ON THIS MAC, IN THE DATA FOLDER", 22, txt3)
}

const trMid = W / 2

func (tr *Trains) render(img *image.RGBA, b *feeds.Board, now time.Time) {
	text(img, pad, pad-8, strings.ToUpper(b.Station), 34, cyan)
	clock := now.Format("15:04")
	text(img, float64(W-pad)-measure(clock, 34), pad-8, clock, 34, amber)
	vrule(img, trMid)
	half := func(x, w int, title string, calls []feeds.TrainCall, dep bool) {
		hx := x + 260
		text(img, float64(hx), pad-2, title, 26, txt2)
		y := 96
		n := 0
		for _, c := range calls {
			if n == 7 {
				break
			}
			when := c.Time
			if !c.Expected.IsZero() {
				when = c.Expected
			}
			if when.Before(now.Add(-time.Minute)) && !c.Cancelled {
				continue // gone
			}
			trainRow(img, x, y, w, c, dep, now)
			y += 50
			n++
		}
		if n == 0 {
			text(img, float64(x), y+10, "NO TRAINS IN THE NEXT TWO HOURS", 24, txt3)
		}
	}
	half(pad, trMid-pad-30, "DEPARTURES", b.Departures, true)
	half(trMid+30, W-pad-trMid-30, "ARRIVALS", b.Arrivals, false)
}

func trainRow(img *image.RGBA, x, y, w int, c feeds.TrainCall, dep bool, now time.Time) {
	text(img, float64(x), y, c.Time.In(now.Location()).Format("15:04"), 30, txt)
	status, sc := "ON TIME", green
	switch {
	case c.Cancelled:
		status, sc = "CANCELLED", hot
	case c.Delayed:
		status, sc = "DELAYED", amber
	case c.Late >= 1:
		status, sc = c.Expected.In(now.Location()).Format("15:04"), amber
		if c.Late >= 15 {
			sc = hot
		}
	}
	statusW := 230.0
	sx := float64(x+w) - measure(status, 28)
	text(img, sx, y+2, status, 28, sc)
	plat := ""
	if c.Platform != "" {
		plat = "P" + c.Platform
	}
	px := float64(x+w) - statusW - 70
	text(img, px, y+2, plat, 28, cyan)
	place := strings.ToUpper(strings.ReplaceAll(c.Place, " International", ""))
	if c.Bus {
		place = "BUS  " + place
	}
	pc := txt
	if c.Cancelled {
		pc = txt3
	}
	text(img, float64(x+120), y, fit(place, 30, px-float64(x+120)-20), 30, pc)
	sub := c.Operator
	subC := txt3
	if c.Reason != "" && (c.Cancelled || c.Late >= 5) {
		sub, subC = c.Reason, amber
	}
	text(img, float64(x+120), y+34, fit(strings.ToUpper(sub), 14, px-float64(x+120)-20), 14, subC)
}
