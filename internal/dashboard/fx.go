package dashboard

import (
	"fmt"
	"image"
	"math"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// FX shows exchange rates against a base currency: each rate large, the move since the working
// day before, and the last month as a line. ECB reference rates via Frankfurter.
type FX struct {
	cfg   func() FXConfig
	fetch func(base string, quotes []string) ([]feeds.Rate, error)
	poll  poller[[]feeds.Rate]
	cache frameCache
}

// FXConfig is fx.txt: the base currency and the ones priced in it.
type FXConfig struct {
	Base   string
	Quotes []string
}

// DefaultFX is what fx.txt holds when there is none.
var DefaultFX = FXConfig{Base: "GBP", Quotes: []string{"USD", "EUR", "JPY", "CHF", "AUD", "CAD"}}

const fxMax = 6

var fxCode = regexp.MustCompile(`^[A-Z]{3}$`)

// ParseFX reads fx.txt's first line: the base, then up to six currencies, as `GBP: USD EUR JPY`
// or `GBP USD, EUR`.
func ParseFX(lines []string) (FXConfig, error) {
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		f := strings.Fields(strings.NewReplacer(":", " ", ",", " ", "/", " ").Replace(strings.ToUpper(l)))
		var c FXConfig
		for i, code := range f {
			if !fxCode.MatchString(code) {
				return FXConfig{}, fmt.Errorf("not a currency code: %s", code)
			}
			if i == 0 {
				c.Base = code
				continue
			}
			dup := code == c.Base
			for _, q := range c.Quotes {
				dup = dup || q == code
			}
			if !dup {
				c.Quotes = append(c.Quotes, code)
			}
		}
		if len(c.Quotes) == 0 {
			return FXConfig{}, fmt.Errorf("name a base and at least one currency, e.g. GBP: USD EUR")
		}
		if len(c.Quotes) > fxMax {
			return FXConfig{}, fmt.Errorf("up to %d currencies", fxMax)
		}
		return c, nil
	}
	return FXConfig{}, fmt.Errorf("empty")
}

// LoadFX reads fx.txt, falling back to DefaultFX.
func LoadFX(path string) FXConfig {
	if b, err := os.ReadFile(path); err == nil {
		if c, err := ParseFX(strings.Split(string(b), "\n")); err == nil {
			return c
		}
	}
	return DefaultFX
}

func NewFX(cfg func() FXConfig) *FX {
	return &FX{cfg: cfg, fetch: feeds.FetchRates, poll: poller[[]feeds.Rate]{every: time.Hour}}
}

func (f *FX) Frame(t, dt float64, now time.Time) *image.RGBA {
	c := f.cfg()
	key := c.Base + ":" + strings.Join(c.Quotes, ",")
	rs, got, ok, err := f.poll.get(key, func(string) ([]feeds.Rate, error) { return f.fetch(c.Base, c.Quotes) })
	if !ok {
		return waitMessage(&f.cache, "EXCHANGE RATES", err)
	}
	return f.cache.get(fmt.Sprint(got.UnixNano()), func(img *image.RGBA) { fxRender(img, c.Base, rs) })
}

// fxFormat gives a rate four significant-ish decimals: 1.3423, 12.345, 204.12.
func fxFormat(v float64) string {
	switch {
	case v >= 100:
		return fmt.Sprintf("%.2f", v)
	case v >= 10:
		return fmt.Sprintf("%.3f", v)
	}
	return fmt.Sprintf("%.4f", v)
}

func fxRender(img *image.RGBA, base string, rs []feeds.Rate) {
	date := ""
	if len(rs) > 0 {
		date = rs[0].Date
	}
	head(img, pad, pad-4, "exchange rates  1 "+base+" buys", W/2)
	r := "ECB REFERENCE RATES  " + date
	text(img, float64(W-pad)-measure(r, 16), pad-2, r, 16, txt3)
	n := max(len(rs), 1)
	colW := (W - 2*pad) / n
	for i, rt := range rs {
		x := pad + i*colW
		w := colW - 40
		if i > 0 {
			wideLine(img, x-20, 80, x-20, H-pad, line, 2)
		}
		text(img, float64(x), 78, rt.Code, 30, cyan)
		v := fxFormat(rt.Rate)
		text(img, float64(x), 118, v, fitPx(v, 64, w, 70), amber)
		ch := 0.0
		if rt.Prev > 0 {
			ch = (rt.Rate/rt.Prev - 1) * 100
		}
		cc, arrow := txt2, "  "
		switch {
		case ch > 0.005:
			cc, arrow = green, "▲ "
		case ch < -0.005:
			cc, arrow = hot, "▼ "
		}
		text(img, float64(x), 200, fmt.Sprintf("%s%+.2f%%", arrow, ch), 22, cc)
		// the month
		y0, y1 := 256, H-pad-26
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, m := range rt.Month {
			lo, hi = math.Min(lo, m), math.Max(hi, m)
		}
		if len(rt.Month) > 1 && hi > lo {
			pts := make([][2]float64, len(rt.Month))
			for k, m := range rt.Month {
				pts[k] = [2]float64{float64(x) + float64(k)*float64(w)/float64(len(rt.Month)-1), float64(y1) - (m-lo)/(hi-lo)*float64(y1-y0)}
			}
			sc := violet
			if first := rt.Month[0]; rt.Rate > first {
				sc = green
			} else if rt.Rate < first {
				sc = hot
			}
			polyline(img, pts, sc, 3)
			disc(img, pts[len(pts)-1][0], pts[len(pts)-1][1], 5, sc)
			text(img, float64(x), y1+6, "LOW "+fxFormat(lo), 16, txt3)
			hl := "HIGH " + fxFormat(hi)
			text(img, float64(x+w)-measure(hl, 16), y1+6, hl, 16, txt3)
		}
	}
}
