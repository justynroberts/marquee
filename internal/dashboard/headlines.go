package dashboard

import (
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// Headlines is a full-resolution news wall: every headline in small type, packed into four
// columns that scroll slowly upwards and loop, so the panel reads like a page of a paper.
// Sources are the news ticker's (Hacker News, or news-sources.txt); Hacker News stories with
// 500+ points are marked in vermilion.
type Headlines struct {
	source func() ([]feeds.Headline, time.Time)
	at     time.Time // when the strips were laid out from
	strips []*image.RGBA
	out    *image.RGBA
	empty  frameCache
	scroll int // pixels scrolled: exactly one per frame, so the motion never stutters
}

const (
	hlCols    = 4
	hlGutter  = 44
	hlMetaPx  = 18
	hlTitlePx = 23
	hlLine    = 30
	hlGap     = 26
	hlFade    = 34
	hlHotPts  = 500
	hlMaxLine = 4
)

var hlColW = (W - 2*pad - (hlCols-1)*hlGutter) / hlCols

// NewHeadlines reads headlines from source, which returns them and when they were fetched.
func NewHeadlines(source func() ([]feeds.Headline, time.Time)) *Headlines {
	return &Headlines{source: source, out: image.NewRGBA(image.Rect(0, 0, W, H))}
}

func (p *Headlines) Frame(t, dt float64, now time.Time) *image.RGBA {
	hs, at := p.source()
	if len(hs) == 0 {
		return p.empty.get("empty", func(img *image.RGBA) {
			msg := "FETCHING HEADLINES…"
			text(img, (W-measure(msg, 30))/2, H/2-20, msg, 30, ink3)
		})
	}
	if p.strips == nil || !at.Equal(p.at) {
		p.layout(hs)
		p.at = at
	}
	fill(p.out, bg)
	p.scroll++
	for i, s := range p.strips {
		p.blit(s, pad+i*(hlColW+hlGutter), i)
	}
	return p.out
}

// layout wraps every headline and deals them to the shortest column, keeping their order.
func (p *Headlines) layout(hs []feeds.Headline) {
	type item struct {
		meta  string
		hot   bool
		lines []string
	}
	cols := make([][]item, hlCols)
	heights := make([]int, hlCols)
	for _, h := range hs {
		meta := strings.ToUpper(h.Source)
		if h.Points > 0 {
			meta += fmt.Sprintf("  %d PTS", h.Points)
		}
		it := item{meta: fit(meta, hlMetaPx, float64(hlColW)), hot: h.Points >= hlHotPts, lines: wrap(h.Title, hlTitlePx, float64(hlColW), hlMaxLine)}
		k := 0
		for j := range heights {
			if heights[j] < heights[k] {
				k = j
			}
		}
		cols[k] = append(cols[k], it)
		heights[k] += itemHeight(len(it.lines))
	}
	p.strips = p.strips[:0]
	for i, items := range cols {
		h := max(heights[i], 1)
		img := image.NewRGBA(image.Rect(0, 0, hlColW, h))
		fill(img, bg)
		y := 0
		for _, it := range items {
			c := ink3
			if it.hot {
				c = hot
			}
			text(img, 0, y, it.meta, hlMetaPx, c)
			y += hlMetaPx + 10
			for _, l := range it.lines {
				text(img, 0, y, l, hlTitlePx, ink)
				y += hlLine
			}
			y += hlGap - (hlLine - hlTitlePx)
			rect(img, 0, y-hlGap/2, 40, y-hlGap/2, rule)
		}
		p.strips = append(p.strips, img)
	}
}

func itemHeight(lines int) int { return hlMetaPx + 10 + lines*hlLine + hlGap - (hlLine - hlTitlePx) }

// blit copies column strip s to x. A strip taller than the panel scrolls (one pixel a frame,
// about 12 px/s on the panel) and wraps round;
// columns start a quarter of their length apart so they never line up. The top and bottom
// edges fade into the ground.
func (p *Headlines) blit(s *image.RGBA, x int, i int) {
	sh := s.Rect.Dy()
	top, view := pad/2, H-pad/2-pad/2
	scroll := sh > view
	off := 0
	if scroll {
		off = (p.scroll + sh*i/hlCols) % sh
	}
	rowBytes := hlColW * 4
	for y := 0; y < view; y++ {
		sy := y + off
		if scroll {
			sy %= sh
		} else if sy >= sh {
			break
		}
		d := p.out.PixOffset(x, top+y)
		src := s.Pix[sy*s.Stride : sy*s.Stride+rowBytes]
		a := 255
		if scroll && y < hlFade {
			a = 255 * y / hlFade
		} else if scroll && view-1-y < hlFade {
			a = 255 * (view - 1 - y) / hlFade
		}
		if a == 255 {
			copy(p.out.Pix[d:d+rowBytes], src)
			continue
		}
		for j := 0; j < rowBytes; j += 4 {
			p.out.Pix[d+j] = blend(uint32(a), bg.R, src[j])
			p.out.Pix[d+j+1] = blend(uint32(a), bg.G, src[j+1])
			p.out.Pix[d+j+2] = blend(uint32(a), bg.B, src[j+2])
		}
	}
}

// wrap breaks s into at most n lines of width w at px, ending with an ellipsis when cut short.
func wrap(s string, px int, w float64, n int) []string {
	var lines []string
	cur := ""
	words := strings.Fields(s)
	for i, word := range words {
		next := word
		if cur != "" {
			next = cur + " " + word
		}
		if measure(next, px) <= w {
			cur = next
			continue
		}
		if cur != "" {
			lines = append(lines, cur)
		}
		cur = word
		if len(lines) == n-1 {
			cur = fit(strings.Join(words[i:], " "), px, w)
			break
		}
	}
	if cur != "" {
		lines = append(lines, fit(cur, px, w))
	}
	return lines
}
