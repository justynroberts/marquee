package dashboard

import (
	"fmt"
	"image"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// OnThisDay is Wikipedia's "on this day": one event at a time, oldest first, its year large on
// the left and the event in mid-size type beside it, with a bar timing the next.
type OnThisDay struct {
	fetch func(time.Month, int) ([]feeds.Event, error)
	poll  poller[[]feeds.Event]
	cache frameCache
}

const (
	otdEvery = 12.0 // seconds per event
	otdLeft  = 560
	otdPx    = 40
)

func NewOnThisDay() *OnThisDay {
	return &OnThisDay{fetch: feeds.FetchOnThisDay, poll: poller[[]feeds.Event]{every: 6 * time.Hour}}
}

func (o *OnThisDay) Frame(t, dt float64, now time.Time) *image.RGBA {
	day := now.Format("01-02")
	evs, _, ok, err := o.poll.get(day, func(string) ([]feeds.Event, error) { return o.fetch(now.Month(), now.Day()) })
	if !ok || len(evs) == 0 {
		return waitMessage(&o.cache, "ON THIS DAY", err)
	}
	i := int(t/otdEvery) % len(evs)
	e := evs[i]
	img := o.cache.get(fmt.Sprint(day, len(evs), i), func(img *image.RGBA) {
		head(img, pad, pad-4, "on this day  "+now.Format("2 January"), otdLeft-pad-40)
		pos := fmt.Sprintf("%d / %d", i+1, len(evs))
		text(img, float64(W-pad)-measure(pos, 22), pad-4, pos, 22, txt2)
		year := fmt.Sprint(e.Year)
		if e.Year <= 0 {
			year = fmt.Sprintf("%d BC", 1-e.Year)
		}
		px := fitPx(year, 170, otdLeft-pad-60, 200)
		b := inkBox(year, px)
		text(img, pad, 150-b.Min.Y, year, px, amber)
		ago := now.Year() - e.Year
		text(img, pad, 150+b.Dy()+30, fmt.Sprintf("%d YEARS AGO", ago), 26, cyan)
		vrule(img, otdLeft-30)
		// the event, as large as fits in five lines
		w := float64(W - pad - otdLeft)
		for px := otdPx; px >= 26; px -= 2 {
			lines := wrap(e.Text, px, w, 99)
			if len(lines) <= int(330/(px*3/2)) || px == 26 {
				lh := px * 3 / 2
				lines = wrap(e.Text, px, w, 330/lh)
				y := 96 + (330-len(lines)*lh)/2
				for j, l := range lines {
					text(img, otdLeft, y+j*lh, l, px, txt)
				}
				break
			}
		}
		rect(img, otdLeft, H-pad, W-pad, H-pad+2, line)
	})
	k := t/otdEvery - float64(int(t/otdEvery))
	rect(img, otdLeft, H-pad, otdLeft+int(float64(W-pad-otdLeft)*k), H-pad+2, cyan)
	return img
}
