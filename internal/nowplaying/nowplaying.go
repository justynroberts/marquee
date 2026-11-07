// Package nowplaying renders a full-resolution (1920x462) "Now Playing" screen for whatever is
// playing in Apple Music or Spotify, in the dashboard's visual language: near-black ground,
// paper ink, SF Mono, hairline rules and one vermilion accent (the playhead).
//
// Layout, left to right: square artwork, then a text column with a small-caps source line, an
// oversized title that marquee-scrolls when it is too long, artist, album, and a progress rule
// with elapsed time left and remaining time right.
//
// Players are polled once a second through osascript. A player that is not running is never
// launched.
package nowplaying

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/justynroberts/marquee/internal/pil"
)

// Layout.
const (
	side    = H - 2*pad       // artwork square
	tx      = pad + side + 48 // text column left edge
	tw      = W - pad - tx    // text column width
	titlePx = 112
	titleX  = tx - 4 // optical alignment of the big glyphs with the text below
	titleY  = 64
	titleH  = 146 // strip height: ascender + descender at titlePx
	artistY = 228
	albumY  = 290
	barY    = 372
	timeY   = H - pad - 30

	scrollSpeed = 90.0 // px/s
	scrollGap   = 224  // px between the end of the title and its repeat
	scrollHold  = 2.5  // s at rest at the start of each pass
)

// state is what the poller last saw.
type state struct {
	track    *Track
	polledAt time.Time // wall time Track.Position was valid
	msg      string    // idle explanation (permission, error)
	art      *image.RGBA
	artGen   int
	lyrics   []lyricLine // synced lyrics for track, nil when none (yet)
	lyrGen   int
}

// View polls the players in the background and renders frames on demand.
type View struct {
	mu sync.Mutex
	st state

	// artwork bookkeeping (poller goroutine only, except artBusy under mu)
	artKey   string
	artTries int
	artLast  time.Time
	artBusy  bool

	lyrKey   string
	lyrTries int
	lyrLast  time.Time
	lyrBusy  bool

	rmu sync.Mutex
	r   renderer // static: artwork, title, artist, progress
	rk  renderer // karaoke: synced lyrics with a sweep

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// New starts polling once a second. Call Close to stop.
func New() *View {
	v := newView()
	go v.loop()
	return v
}

func newView() *View {
	ctx, cancel := context.WithCancel(context.Background())
	return &View{ctx: ctx, cancel: cancel, done: make(chan struct{})}
}

// Close stops polling. Safe to call more than once.
func (v *View) Close() {
	v.cancel()
}

func (v *View) loop() {
	defer close(v.done)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		v.pollOnce()
		select {
		case <-v.ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (v *View) pollOnce() {
	ctx, cancel := context.WithTimeout(v.ctx, 2*time.Second)
	p, err := fetch(ctx)
	cancel()
	if v.ctx.Err() != nil {
		return
	}
	if err != nil {
		p = poll{Errs: []playerError{{Source: "Music", Msg: err.Error()}}}
	}
	v.apply(p, time.Now())
}

// apply records a poll result and kicks off an artwork fetch when the track changed.
func (v *View) apply(p poll, at time.Time) {
	t, ok := p.pick()
	v.mu.Lock()
	if !ok {
		v.st.track = nil
		v.st.msg = p.idleMessage()
		v.mu.Unlock()
		return
	}
	v.st.track, v.st.polledAt, v.st.msg = &t, at, ""
	k := t.key()
	if k != v.artKey {
		v.artKey, v.artTries = k, 0
		v.st.art = nil
		v.st.artGen++
	}
	// Fetch once per track; retry a failure (e.g. Spotify's URL arriving late) a couple of times.
	want := v.st.art == nil && !v.artBusy && v.artTries < 3 && (v.artTries == 0 || at.Sub(v.artLast) > 8*time.Second)
	if want {
		v.artBusy, v.artLast = true, at
		v.artTries++
	}
	if k != v.lyrKey {
		v.lyrKey, v.lyrTries = k, 0
		v.st.lyrics = nil
		v.st.lyrGen++
	}
	// Lyrics: once per track; a network failure is retried after 30 s, at most three times.
	wantLyr := v.st.lyrics == nil && !v.lyrBusy && v.lyrTries < 3 && (v.lyrTries == 0 || at.Sub(v.lyrLast) > 30*time.Second)
	if wantLyr {
		v.lyrBusy, v.lyrLast = true, at
		v.lyrTries++
	}
	v.mu.Unlock()
	if want {
		go v.loadArt(t, k)
	}
	if wantLyr {
		go v.loadLyrics(t, k)
	}
}

func (v *View) loadLyrics(t Track, k string) {
	ctx, cancel := context.WithTimeout(v.ctx, 12*time.Second)
	lines, err := fetchLyrics(ctx, t)
	cancel()
	v.mu.Lock()
	defer v.mu.Unlock()
	v.lyrBusy = false
	if k != v.lyrKey {
		return
	}
	if errors.Is(err, errNoLyrics) {
		v.lyrTries = 3
		return
	}
	if err == nil && len(lines) > 0 {
		v.st.lyrics = lines
		v.st.lyrGen++
		v.lyrTries = 3
	}
}

func (v *View) loadArt(t Track, k string) {
	ctx, cancel := context.WithTimeout(v.ctx, 5*time.Second)
	img, err := fetchArt(ctx, t)
	cancel()
	var scaled *image.RGBA
	if err == nil && img != nil {
		scaled = squareArt(img, side)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.artBusy = false
	if k != v.artKey {
		return
	}
	if err == nil && img == nil {
		v.artTries = 3 // the track genuinely has no artwork
	}
	if scaled != nil {
		v.st.art = scaled
		v.st.artGen++
		v.artTries = 3
	}
}

// squareArt centre-crops img to a square and Lanczos-scales it to n x n, composited on bg.
func squareArt(img image.Image, n int) *image.RGBA {
	b := img.Bounds()
	s := min(b.Dx(), b.Dy())
	if s <= 0 {
		return nil
	}
	x0, y0 := b.Min.X+(b.Dx()-s)/2, b.Min.Y+(b.Dy()-s)/2
	src := image.NewNRGBA(image.Rect(0, 0, s, s))
	draw.Draw(src, src.Rect, img, image.Pt(x0, y0), draw.Src)
	out := pil.ResizeRGBA(pil.Image{W: s, H: s, Pix: src.Pix}, n, n)
	dst := image.NewRGBA(image.Rect(0, 0, n, n))
	for i := 0; i < len(out.Pix); i += 4 {
		a := uint32(out.Pix[i+3])
		dst.Pix[i] = blend(a, bg.R, out.Pix[i])
		dst.Pix[i+1] = blend(a, bg.G, out.Pix[i+1])
		dst.Pix[i+2] = blend(a, bg.B, out.Pix[i+2])
		dst.Pix[i+3] = 255
	}
	return dst
}

// Frame returns the screen at time now. The image is shared and may be returned again by later
// calls: treat it as read-only.
func (v *View) Frame(now time.Time) *image.RGBA {
	v.mu.Lock()
	st := v.st
	v.mu.Unlock()
	v.rmu.Lock()
	defer v.rmu.Unlock()
	st.lyrics, st.lyrGen = nil, 0 // the static view never shows lyrics
	return v.r.frame(st, now)
}

// Karaoke returns the lyrics screen: the current line sweeps from dim to bright as it is
// sung, the next line beneath. Without synced lyrics it shows the static screen.
func (v *View) Karaoke(now time.Time) *image.RGBA {
	v.mu.Lock()
	st := v.st
	v.mu.Unlock()
	v.rmu.Lock()
	defer v.rmu.Unlock()
	return v.rk.frame(st, now)
}

// Lyric reports the lyric showing at now for other screens (the HUD): the current line (the
// title before the first line), the next line, and how far through the current line the
// singer is (0..1). ok is false without a track or synced lyrics.
func (v *View) Lyric(now time.Time) (cur, next string, sweep float64, ok bool) {
	v.mu.Lock()
	st := v.st
	v.mu.Unlock()
	if st.track == nil || st.lyrics == nil {
		return "", "", 0, false
	}
	pos := position(st, now)
	i := lineAt(st.lyrics, pos)
	cur, sweep = st.track.Title, 1 // intro: the title, fully lit
	if i >= 0 {
		cur, sweep = st.lyrics[i].text, lineSweep(st.lyrics, i, pos)
		if cur == "" {
			cur = "· · ·"
		}
	}
	for j := i + 1; j < len(st.lyrics); j++ {
		if st.lyrics[j].text != "" {
			next = st.lyrics[j].text
			break
		}
	}
	return cur, next, sweep, true
}

// position is the playhead at now, interpolated from the last poll.
func position(st state, now time.Time) float64 {
	t := st.track
	pos := t.Position
	if t.Playing {
		pos += now.Sub(st.polledAt).Seconds()
	}
	if t.Duration > 0 {
		pos = math.Min(pos, t.Duration)
	}
	return math.Max(pos, 0)
}

// lineSweep is how far through line i the singer is: the sweep runs over most of the gap to
// the next timestamp (singers finish a line before the next starts).
func lineSweep(lines []lyricLine, i int, pos float64) float64 {
	start := lines[i].t
	end := start + 4
	if i+1 < len(lines) {
		end = lines[i+1].t
	}
	dur := math.Max(0.5, math.Min((end-start)*0.85, 8))
	return math.Max(0, math.Min(1, (pos+Lead()-start)/dur))
}

// ---------------------------------------------------------------- render

type baseKey struct {
	idle                      bool
	msg                       string
	key, title, artist, album string
	source                    string
	playing                   bool
	artGen                    int
	lyrGen                    int
}

type dynKey struct {
	elapsed, remaining int
	barW               int
	scroll             int
	hasDur             bool
	line               int // lyric line index (-1 before the first); -2 without lyrics
	sweep              int // karaoke sweep through the current line, 0..400
}

type renderer struct {
	bk     baseKey
	base   *image.RGBA
	dk     dynKey
	last   *image.RGBA
	strip  *image.RGBA // title strip for scrolling (nil when it fits)
	period int         // strip width = title width + gap
	epoch  time.Time   // scroll origin for the current track
}

func (r *renderer) frame(st state, now time.Time) *image.RGBA {
	var bk baseKey
	if st.track == nil {
		bk = baseKey{idle: true, msg: st.msg}
	} else {
		t := st.track
		bk = baseKey{key: t.key(), title: t.Title, artist: t.Artist, album: t.Album, source: t.Source, playing: t.Playing, artGen: st.artGen, lyrGen: st.lyrGen}
	}
	if r.base == nil || bk != r.bk {
		if r.base == nil || bk.key != r.bk.key || bk.title != r.bk.title {
			r.epoch = now
		}
		r.bk = bk
		r.base = r.drawBase(st)
		r.last = nil
	}
	if st.track == nil {
		if r.last == nil {
			r.last = r.base
		}
		return r.last
	}

	t := st.track
	pos := position(st, now)
	dk := dynKey{elapsed: int(pos), hasDur: t.Duration > 0, line: -2}
	if st.lyrics != nil {
		dk.line = lineAt(st.lyrics, pos)
		if dk.line >= 0 && t.Playing {
			dk.sweep = int(lineSweep(st.lyrics, dk.line, pos) * 400)
		} else if dk.line >= 0 {
			dk.sweep = 400 // paused: show the line fully lit
		}
	}
	if dk.hasDur {
		dk.remaining = int(math.Ceil(t.Duration - pos))
		dk.barW = int(float64(tw) * pos / t.Duration)
	}
	if r.strip != nil {
		dk.scroll = r.scrollOffset(now)
	}
	if r.last != nil && dk == r.dk {
		return r.last
	}
	r.dk = dk

	img := image.NewRGBA(r.base.Rect)
	copy(img.Pix, r.base.Pix)
	if r.strip != nil {
		r.blitStrip(img, dk.scroll)
	}
	if st.lyrics != nil {
		drawLyrics(img, t, st.lyrics, dk.line, float64(dk.sweep)/400)
	}
	r.drawProgress(img, t, dk)
	r.last = img
	return img
}

// scrollOffset: hold at 0, then move left at constant speed until the repeat lands at 0.
func (r *renderer) scrollOffset(now time.Time) int {
	travel := float64(r.period) / scrollSpeed
	cycle := scrollHold + travel
	t := math.Mod(math.Max(now.Sub(r.epoch).Seconds(), 0), cycle)
	if t < scrollHold {
		return 0
	}
	return int((t-scrollHold)*scrollSpeed) % r.period
}

func (r *renderer) blitStrip(img *image.RGBA, off int) {
	sw := r.period
	for y := 0; y < titleH; y++ {
		so := y * r.strip.Stride
		do := img.PixOffset(titleX, titleY+y)
		x := 0
		for x < tw {
			sx := (off + x) % sw
			n := min(tw-x, sw-sx)
			copy(img.Pix[do+x*4:do+(x+n)*4], r.strip.Pix[so+sx*4:so+(sx+n)*4])
			x += n
		}
	}
}

func clock(sec int) string {
	if sec >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", sec/3600, sec/60%60, sec%60)
	}
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

func (r *renderer) drawProgress(img *image.RGBA, t *Track, dk dynKey) {
	fillCol, tick := ink, hot
	if !t.Playing {
		fillCol, tick = ink3, ink3
	}
	rect(img, tx, barY, W-pad, barY+5, rule)
	if dk.hasDur {
		if dk.barW > 0 {
			rect(img, tx, barY, tx+dk.barW, barY+5, fillCol)
		}
		hx := min(tx+dk.barW, W-pad-4)
		rect(img, hx, barY-9, hx+3, barY+14, tick)
	}
	text(img, float64(tx), timeY, clock(dk.elapsed), 26, ink2)
	if dk.hasDur {
		rem := "-" + clock(dk.remaining)
		text(img, float64(W-pad)-measure(rem, 26), timeY, rem, 26, ink3)
	}
}

func (r *renderer) drawBase(st state) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	fill(img, bg)
	r.strip, r.period = nil, 0
	if st.track == nil {
		drawIdle(img, st.msg)
		return img
	}
	t := st.track

	// artwork
	if st.art != nil {
		draw.Draw(img, image.Rect(pad, pad, pad+side, pad+side), st.art, image.Point{}, draw.Src)
	} else {
		rect(img, pad, pad, pad+side-1, pad+side-1, rule)
		caps(img, pad+24, pad+side-24-22, "no artwork", side-48, ink3)
	}

	if st.lyrics != nil {
		drawLyricsHeader(img, t)
		return img
	}

	// source line
	src := strings.ToUpper(t.Source)
	x := caps(img, tx, pad, src, tw, ink3)
	if !t.Playing {
		text(img, float64(tx)+x, pad, "  ·  PAUSED", 22, ink2)
	}

	// title: static when it fits, otherwise a strip that Frame scrolls
	title := t.Title
	if title == "" {
		title = "Untitled"
	}
	if w := measure(title, titlePx); w <= float64(tw) {
		text(img, titleX, titleY, title, titlePx, ink)
	} else {
		r.period = int(math.Ceil(w)) + scrollGap
		r.strip = image.NewRGBA(image.Rect(0, 0, r.period, titleH))
		fill(r.strip, bg)
		text(r.strip, 0, 0, title, titlePx, ink)
	}

	text(img, float64(tx), artistY, fit(t.Artist, 44, float64(tw)), 44, ink2)
	text(img, float64(tx), albumY, fit(t.Album, 30, float64(tw)), 30, ink3)
	return img
}

func drawIdle(img *image.RGBA, msg string) {
	caps(img, pad, pad, "Now playing", W-2*pad, ink3)
	text(img, float64(pad-4), 150, "Nothing playing", 96, ink2)
	if msg == "" {
		text(img, float64(pad), 290, "Play something in Music or Spotify", 30, ink3)
		return
	}
	rect(img, pad, 302, pad+11, 313, hot)
	text(img, float64(pad+28), 290, fit(msg, 30, float64(W-2*pad-28)), 30, ink2)
}

// ---------------------------------------------------------------- lyrics layout

const (
	lyricTop  = 78  // top of the lyric block
	lyricBot  = 352 // the block (current line or two, then the next line) ends above this
	nextLyrPx = 36
)

// Current line: one line as large as fits, else two lines; never truncated unless even two
// lines at the smallest size cannot hold it.
var (
	oneLineSizes = []int{84, 76, 68}
	twoLineSizes = []int{68, 60, 52, 46, 40}
)

// wrapLyric splits s into lines no wider than w at px, at most max lines; ok is false if it
// does not fit.
func wrapLyric(s string, px, max int, w float64) ([]string, bool) {
	var lines []string
	cur := ""
	for _, word := range strings.Fields(s) {
		try := strings.TrimSpace(cur + " " + word)
		if measureBold(try, px) <= w {
			cur = try
			continue
		}
		if cur == "" || measureBold(word, px) > w {
			return nil, false
		}
		lines = append(lines, cur)
		cur = word
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines, len(lines) <= max
}

func layoutLyric(s string, w float64) ([]string, int) {
	for _, px := range oneLineSizes {
		if measureBold(s, px) <= w {
			return []string{s}, px
		}
	}
	for _, px := range twoLineSizes {
		if ls, ok := wrapLyric(s, px, 2, w); ok {
			// Balance the two lines so the second is not a lone word.
			if len(ls) == 2 {
				ls = balance(s, px, w, ls)
			}
			return ls, px
		}
	}
	px := twoLineSizes[len(twoLineSizes)-1]
	ls, _ := wrapLyric(s, px, 99, w)
	if len(ls) > 2 {
		ls = []string{ls[0], fitBold(strings.Join(ls[1:], " "), px, w)}
	}
	return ls, px
}

// balance moves the break point to make the two lines as even as possible.
func balance(s string, px int, w float64, ls []string) []string {
	words := strings.Fields(s)
	best, bestD := ls, math.Inf(1)
	for i := 1; i < len(words); i++ {
		a, b := strings.Join(words[:i], " "), strings.Join(words[i:], " ")
		wa, wb := measureBold(a, px), measureBold(b, px)
		if wa > w || wb > w {
			continue
		}
		if d := math.Abs(wa - wb); d < bestD {
			best, bestD = []string{a, b}, d
		}
	}
	return best
}

func fitBold(s string, px int, w float64) string {
	if measureBold(s, px) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && measureBold(string(r)+"…", px) > w {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

func drawLyricsHeader(img *image.RGBA, t *Track) {
	x := caps(img, tx, pad, strings.ToUpper(t.Source), tw, ink3)
	state := ""
	if !t.Playing {
		state = "PAUSED  ·  "
	}
	rest := "  ·  " + state + t.Title
	if t.Artist != "" {
		rest += " — " + t.Artist
	}
	text(img, float64(tx)+x, pad, fit(rest, 22, float64(tw)-x), 22, ink2)
}

func drawLyrics(img *image.RGBA, t *Track, lines []lyricLine, i int, sweep float64) {
	cur, curCol, karaoke := "", ink, i >= 0
	switch {
	case i < 0:
		cur, curCol = t.Title, ink2 // intro: the title until the first line
	case lines[i].text == "":
		cur, curCol, karaoke = "· · ·", ink3, false // instrumental break
	default:
		cur = lines[i].text
	}
	ls, px := layoutLyric(cur, float64(tw))
	lh := int(float64(px) * 1.18)
	y := lyricTop
	if len(ls) == 1 {
		y += (84 - px) / 2
	}
	// The sweep runs across the lines in reading order, by width.
	total := 0.0
	for _, l := range ls {
		total += measureBold(l, px)
	}
	lit := sweep * total
	for _, l := range ls {
		if !karaoke {
			textBold(img, titleX, y, l, px, curCol)
		} else {
			w := measureBold(l, px)
			textBold(img, titleX, y, l, px, sung)
			if lit > 0 {
				textBoldClip(img, titleX, y, l, px, ink, titleX+math.Min(lit, w))
			}
			lit -= w
		}
		y += lh
	}
	// The next line, if there is room for it above the progress bar.
	ny := max(y+14, 240)
	if ny+nextLyrPx > lyricBot {
		return
	}
	for j := i + 1; j < len(lines); j++ {
		if lines[j].text != "" {
			text(img, float64(tx), ny, fit(lines[j].text, nextLyrPx, float64(tw)), nextLyrPx, ink3)
			break
		}
	}
}

// Current returns the track the players report now, if any.
func (v *View) Current() (title, artist string, playing bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.st.track == nil {
		return "", "", false
	}
	return v.st.track.Title, v.st.track.Artist, v.st.track.Playing
}
