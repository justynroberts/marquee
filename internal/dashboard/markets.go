package dashboard

import (
	"fmt"
	"image"
	"math"
	"os"
	"strings"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// Markets is cryptocurrency prices from CoinGecko, one column per coin: the price as large as
// the column allows, the day's move, and the last seven days as a line between its low and high.
// Rises are green and falls vermilion.
type Markets struct {
	cfg   func() MarketsConfig
	fetch func([]string, string) ([]feeds.Coin, error)
	poll  poller[[]feeds.Coin]
	cache frameCache
}

// MarketsConfig is markets.txt: CoinGecko coin ids, one per line (up to four), and an optional
// "currency = gbp" line.
type MarketsConfig struct {
	Coins    []string
	Currency string
}

var DefaultCoins = []string{"bitcoin", "ethereum", "solana"}

const maxCoins = 4

// LoadMarkets reads path, defaulting to bitcoin, ethereum and solana in US dollars.
func LoadMarkets(path string) MarketsConfig {
	b, _ := os.ReadFile(path)
	return ParseMarkets(string(b))
}

// ParseMarkets reads markets.txt's text.
func ParseMarkets(text string) MarketsConfig {
	c := MarketsConfig{Currency: "usd"}
	for _, l := range strings.Split(text, "\n") {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if k, v, ok := strings.Cut(l, "="); ok && strings.TrimSpace(k) == "currency" {
			c.Currency = strings.TrimSpace(v)
			continue
		}
		if len(c.Coins) < maxCoins {
			c.Coins = append(c.Coins, strings.Join(strings.Fields(l), "-"))
		}
	}
	if len(c.Coins) == 0 {
		c.Coins = DefaultCoins
	}
	return c
}

func NewMarkets(cfg func() MarketsConfig) *Markets {
	return &Markets{cfg: cfg, fetch: feeds.FetchCoins, poll: poller[[]feeds.Coin]{every: 2 * time.Minute}}
}

func currencySign(cur string) string {
	switch cur {
	case "usd":
		return "$"
	case "gbp":
		return "£"
	case "eur":
		return "€"
	case "jpy":
		return "¥"
	}
	return strings.ToUpper(cur) + " "
}

// money formats a price with thousands separators and sensible decimals.
func money(v float64) string {
	switch {
	case v >= 1000:
		s := fmt.Sprintf("%.0f", v)
		var b strings.Builder
		for i, r := range s {
			if i > 0 && (len(s)-i)%3 == 0 {
				b.WriteByte(',')
			}
			b.WriteRune(r)
		}
		return b.String()
	case v >= 1:
		return fmt.Sprintf("%.2f", v)
	case v > 0:
		return fmt.Sprintf("%.*f", int(math.Max(4, 3-math.Floor(math.Log10(v)))), v)
	}
	return "0"
}

func (m *Markets) Frame(t, dt float64, now time.Time) *image.RGBA {
	cfg := m.cfg()
	key := cfg.Currency + "|" + strings.Join(cfg.Coins, ",")
	coins, got, ok, err := m.poll.get(key, func(string) ([]feeds.Coin, error) { return m.fetch(cfg.Coins, cfg.Currency) })
	if !ok {
		return waitMessage(&m.cache, "PRICES", err)
	}
	return m.cache.get(fmt.Sprint(key, got.UnixNano()), func(img *image.RGBA) { m.render(img, coins, cfg.Currency, got) })
}

func (m *Markets) render(img *image.RGBA, coins []feeds.Coin, cur string, got time.Time) {
	head(img, pad, pad-4, "markets  "+cur, 600)
	upd := "COINGECKO   " + got.Format("15:04")
	text(img, float64(W-pad)-measure(upd, 22), pad-4, upd, 22, txt3)
	n := len(coins)
	gut := 60
	cw := (W - 2*pad - (n-1)*gut) / n
	sign := currencySign(cur)
	for i, c := range coins {
		x := pad + i*(cw+gut)
		if i > 0 {
			vrule(img, x-gut/2)
		}
		text(img, float64(x), 84, c.Symbol, 30, cyan)
		text(img, float64(x)+measure(c.Symbol, 30)+16, 92, fit(strings.ToUpper(c.Name), 20, float64(cw)-measure(c.Symbol, 30)-16), 20, txt3)
		price := sign + money(c.Price)
		px := fitPx(price, 96, cw, 110)
		b := inkBox(price, px)
		text(img, float64(x), 136-b.Min.Y, price, px, txt)
		y := 136 + b.Dy() + 22
		// up green, down vermilion: the day's move is the reading that matters
		col := green
		if c.Change24h < 0 {
			col = hot
		}
		chg := fmt.Sprintf("%+.2f%%  24H", c.Change24h)
		text(img, float64(x), y, chg, 28, col)
		hl := fmt.Sprintf("H %s  L %s", money(c.High24h), money(c.Low24h))
		text(img, float64(x+cw)-measure(hl, 18), y+6, hl, 18, txt3)

		// seven days
		sy0, sy1 := y+56, H-pad-22
		if len(c.Week) >= 2 {
			lo, hi := math.Inf(1), math.Inf(-1)
			for _, v := range c.Week {
				lo, hi = math.Min(lo, v), math.Max(hi, v)
			}
			if hi == lo {
				hi = lo + 1
			}
			pts := make([][2]float64, len(c.Week))
			for j, v := range c.Week {
				pts[j] = [2]float64{float64(x) + float64(j)/float64(len(c.Week)-1)*float64(cw), float64(sy1) - (v-lo)/(hi-lo)*float64(sy1-sy0)}
			}
			rect(img, x, sy1, x+cw, sy1, line)
			wk := green
			if c.Week[len(c.Week)-1] < c.Week[0] {
				wk = hot
			}
			polyline(img, pts, wk, 2)
			last := pts[len(pts)-1]
			disc(img, last[0], last[1], 5, txt)
			text(img, float64(x), sy1+6, "7 DAYS", 16, txt3)
			rng := fmt.Sprintf("%s – %s", money(lo), money(hi))
			text(img, float64(x+cw)-measure(rng, 16), sy1+6, rng, 16, txt3)
		}
	}
}
