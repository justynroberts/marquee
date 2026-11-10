package dashboard

import (
	"context"
	"fmt"
	"image"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// Newsreel is one news source as a full-resolution crawl: the source named across the top and
// its headlines in mid-size type on two rows sliding left, alternate stories on each row. A new
// fetch is swapped in when a row comes round to its start, so nothing jumps mid-sentence.
type Newsreel struct {
	source func() string // a preset name or feed URL, re-read on each refresh
	fetch  func(feeds.Source) ([]feeds.Headline, error)

	mu    sync.Mutex
	hs    []feeds.Headline
	label string
	err   error
	asked string
	at    time.Time
	busy  bool
	gen   int // bumped on each successful fetch

	shown int // gen the rows were built from
	rows  [2]reelRow
	cache frameCache
}

type reelItem struct {
	title string
	hot   bool
	w     int // title plus separator
}

type reelRow struct {
	items, pending []reelItem
	pos            int // pixels scrolled into items
	tiles          map[int]*image.RGBA
}

const (
	reelPx      = 76
	reelRowH    = 150
	reelTop     = 128 // first row
	reelSep     = 150 // gap between stories, with a marker in the middle
	reelSpeed   = 8   // pixels a frame: whole pixels, so the crawl never stutters
	reelFade    = 70
	reelEvery   = 10 * time.Minute
	reelStories = 30
)

// NewNewsreel crawls the source named by source() (see feeds.LookupSource).
func NewNewsreel(source func() string) *Newsreel {
	return &Newsreel{source: source, fetch: func(s feeds.Source) ([]feeds.Headline, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return s.Fetch(ctx, reelStories)
	}}
}

func (p *Newsreel) refresh() {
	name := p.source()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.busy || (name == p.asked && time.Since(p.at) < reelEvery && (p.hs != nil || time.Since(p.at) < time.Minute)) {
		return
	}
	src, ok := feeds.LookupSource(name)
	if !ok {
		p.asked, p.at, p.hs = name, time.Now(), nil
		p.err = fmt.Errorf("unknown source %q: use %s or a feed URL", name, presetKeys())
		return
	}
	if name != p.asked {
		p.hs = nil // a different source: show "fetching" rather than the old one
	}
	p.busy, p.asked = true, name
	go func() {
		hs, err := p.fetch(src)
		p.mu.Lock()
		defer p.mu.Unlock()
		p.busy, p.at = false, time.Now()
		if err != nil {
			p.err = err
			return
		}
		p.hs, p.err, p.gen = hs, nil, p.gen+1
		p.label = src.Label
		if p.label == "" && len(hs) > 0 {
			p.label = shortLabel(hs[0].Source)
		}
	}()
}

func presetKeys() string {
	var ks []string
	for _, s := range feeds.Presets {
		ks = append(ks, s.Key)
	}
	return strings.Join(ks, ", ")
}

// shortLabel trims a feed's own title ("Ars Technica - All content") to its name.
func shortLabel(s string) string {
	for _, sep := range []string{" | ", " – ", " — ", " - ", ": "} {
		if i := strings.Index(s, sep); i > 0 {
			s = s[:i]
		}
	}
	return strings.TrimSpace(s)
}

func (p *Newsreel) Frame(t, dt float64, now time.Time) *image.RGBA {
	p.refresh()
	p.mu.Lock()
	hs, label, err, gen := p.hs, p.label, p.err, p.gen
	p.mu.Unlock()
	if hs == nil {
		msg := "FETCHING HEADLINES…"
		if err != nil {
			msg = strings.ToUpper(err.Error())
		}
		return p.cache.get("msg"+msg, func(img *image.RGBA) {
			msg = fit(msg, 28, W-2*pad)
			text(img, (W-measure(msg, 28))/2, H/2-18, msg, 28, ink3)
		})
	}
	if gen != p.shown {
		p.load(hs)
		p.shown = gen
	}
	img := p.cache.get(fmt.Sprint(label, len(hs), now.Format("15:04")), func(img *image.RGBA) {
		text(img, pad, pad-6, strings.ToUpper(label), 34, ink)
		right := fmt.Sprintf("%d STORIES   %s", len(hs), now.Format("15:04"))
		text(img, float64(W-pad)-measure(right, 26), pad-2, right, 26, ink3)
		rect(img, pad, 96, W-pad, 97, rule)
	})
	for i := range p.rows {
		p.rows[i].advance()
		p.rows[i].draw(img, reelTop+i*reelRowH)
	}
	return img
}

// load deals the headlines to the rows alternately. The first load starts the rows at once
// (the second a third of a screen behind, so their gaps never line up); later ones wait for
// each row to come round.
func (p *Newsreel) load(hs []feeds.Headline) {
	var lists [2][]reelItem
	for i, h := range hs {
		title := strings.Join(strings.Fields(h.Title), " ")
		if title == "" {
			continue
		}
		w := int(math.Ceil(measure(title, reelPx))) + reelSep
		lists[i%2] = append(lists[i%2], reelItem{title: title, hot: h.Points >= hlHotPts, w: w})
	}
	if len(lists[1]) == 0 {
		lists[1] = lists[0]
	}
	for i := range p.rows {
		r := &p.rows[i]
		if r.items == nil {
			r.items, r.tiles = lists[i], map[int]*image.RGBA{}
			r.pos = -W / 3 * i
		} else {
			r.pending = lists[i]
		}
	}
}

func total(items []reelItem) int {
	n := 0
	for _, it := range items {
		n += it.w
	}
	return n
}

func (r *reelRow) advance() {
	r.pos += reelSpeed
	if tot := total(r.items); r.pos >= tot {
		r.pos -= tot
		if r.pending != nil {
			r.items, r.pending, r.tiles = r.pending, nil, map[int]*image.RGBA{}
			r.pos = 0
		}
	}
}

// draw copies the visible stories into img at row top y, rendering each story's tile when it
// first comes on screen and dropping it once it has gone.
func (r *reelRow) draw(img *image.RGBA, y int) {
	if len(r.items) == 0 {
		return
	}
	// find the story at the left edge
	i, x := 0, -r.pos
	for x+r.items[i].w <= 0 {
		x += r.items[i].w
		i = (i + 1) % len(r.items)
	}
	used := map[int]bool{}
	for n := 0; x < W && n < 64; n++ {
		it := r.items[i]
		tile := r.tiles[i]
		if tile == nil {
			tile = reelTile(it)
			r.tiles[i] = tile
		}
		used[i] = true
		blitX(img, tile, x, y)
		x += it.w
		i = (i + 1) % len(r.items)
	}
	for k := range r.tiles {
		if !used[k] {
			delete(r.tiles, k)
		}
	}
	// soften both ends
	for yy := y; yy < y+reelRowH; yy++ {
		for k := 0; k < reelFade; k++ {
			a := uint32(255 * k / reelFade)
			for _, xx := range [2]int{k, W - 1 - k} {
				o := img.PixOffset(xx, yy)
				img.Pix[o] = blend(a, bg.R, img.Pix[o])
				img.Pix[o+1] = blend(a, bg.G, img.Pix[o+1])
				img.Pix[o+2] = blend(a, bg.B, img.Pix[o+2])
			}
		}
	}
}

// reelTile is one story: its title vertically centred, then the gap with a small square
// marker, vermilion for a Hacker News story with 500+ points.
func reelTile(it reelItem) *image.RGBA {
	t := image.NewRGBA(image.Rect(0, 0, it.w, reelRowH))
	fill(t, bg)
	b := inkBox("ÁHgjy", reelPx)
	ty := (reelRowH-b.Dy())/2 - b.Min.Y
	text(t, 0, ty, it.title, reelPx, ink)
	c := ink3
	if it.hot {
		c = hot
	}
	mx, my := it.w-reelSep/2, reelRowH/2
	rect(t, mx-7, my-7, mx+7, my+7, c)
	return t
}

// blitX copies tile to img at (x, y), clipped to the panel's width.
func blitX(img, tile *image.RGBA, x, y int) {
	x0, x1 := max(x, 0), min(x+tile.Rect.Dx(), W)
	if x0 >= x1 {
		return
	}
	n := (x1 - x0) * 4
	for j := 0; j < tile.Rect.Dy(); j++ {
		s := tile.PixOffset(x0-x, j)
		d := img.PixOffset(x0, y+j)
		copy(img.Pix[d:d+n], tile.Pix[s:s+n])
	}
}
