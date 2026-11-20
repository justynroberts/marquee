package dashboard

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// FocusConfig is a pomodoro rhythm: Work minutes of focus, Break minutes off, and a Long break
// after every Every sessions.
type FocusConfig struct {
	Work, Break, Long time.Duration
	Every             int
}

// DefaultFocus is 25 minutes on, 5 off, and 15 off after every fourth session.
var DefaultFocus = FocusConfig{Work: 25 * time.Minute, Break: 5 * time.Minute, Long: 15 * time.Minute, Every: 4}

// ParseFocus reads "work break [long [every]]": minutes as plain numbers ("25 5", "25/5",
// "50 10 30 4") or durations ("25m 5m", "90s"). Missing values take the defaults.
func ParseFocus(s string) (FocusConfig, error) {
	f := strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == '/' || r == ',' })
	if len(f) < 2 || len(f) > 4 {
		return FocusConfig{}, errors.New("give focus and break minutes, e.g. 25 5 (optionally a long break and how often: 25 5 15 4)")
	}
	c := DefaultFocus
	dur := func(v string) (time.Duration, error) {
		v = strings.ToLower(v)
		if s, ok := strings.CutSuffix(v, "min"); ok { // 25min
			v = s + "m"
		}
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return time.Duration(n * float64(time.Minute)), nil
		}
		if strings.HasSuffix(v, "m") || strings.HasSuffix(v, "s") || strings.HasSuffix(v, "h") {
			return time.ParseDuration(v)
		}
		return 0, fmt.Errorf("%q is not a number of minutes", v)
	}
	var err error
	if c.Work, err = dur(f[0]); err != nil {
		return FocusConfig{}, err
	}
	if c.Break, err = dur(f[1]); err != nil {
		return FocusConfig{}, err
	}
	if len(f) > 2 {
		if c.Long, err = dur(f[2]); err != nil {
			return FocusConfig{}, err
		}
	} else {
		c.Long = max(DefaultFocus.Long, 3*c.Break)
	}
	if len(f) > 3 {
		if c.Every, err = strconv.Atoi(f[3]); err != nil {
			return FocusConfig{}, fmt.Errorf("%q is not a number of sessions", f[3])
		}
	}
	switch {
	case c.Work < time.Minute || c.Work > 3*time.Hour:
		return FocusConfig{}, errors.New("focus must be 1 to 180 minutes")
	case c.Break < time.Minute || c.Break > time.Hour:
		return FocusConfig{}, errors.New("a break must be 1 to 60 minutes")
	case c.Long < time.Minute || c.Long > 2*time.Hour:
		return FocusConfig{}, errors.New("a long break must be 1 to 120 minutes")
	case c.Every < 1 || c.Every > 12:
		return FocusConfig{}, errors.New("the long break comes every 1 to 12 sessions")
	}
	return c, nil
}

// LoadFocus reads the first non-comment line of path, or gives DefaultFocus.
func LoadFocus(path string) FocusConfig {
	b, err := os.ReadFile(path)
	if err != nil {
		return DefaultFocus
	}
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if c, err := ParseFocus(l); err == nil {
			return c
		}
		break
	}
	return DefaultFocus
}

type fcKind int

const (
	fcWork fcKind = iota
	fcBreak
	fcLong
)

func (k fcKind) name() string { return [...]string{"FOCUS", "BREAK", "LONG BREAK"}[k] }
func (k fcKind) colour() color.RGBA {
	return [...]color.RGBA{amber, green, cyan}[k]
}

func (c FocusConfig) length(k fcKind) float64 {
	return [...]time.Duration{c.Work, c.Break, c.Long}[k].Seconds()
}

// fcPhase is where el seconds into a run of sessions falls: the phase's index (from 0), kind,
// start and end (seconds from the run's start), which session of the cycle it belongs to
// (from 0) and how many focus sessions have finished.
type fcPhase struct {
	idx        int
	kind       fcKind
	start, end float64
	session    int
	done       int
}

func fcPhaseAt(c FocusConfig, el float64) fcPhase {
	cycle := float64(c.Every)*c.length(fcWork) + float64(c.Every-1)*c.length(fcBreak) + c.length(fcLong)
	n := int(el / cycle)
	at := float64(n) * cycle
	p := fcPhase{idx: n * 2 * c.Every, done: n * c.Every}
	for i := 0; ; i++ {
		k := fcWork
		if i%2 == 1 {
			k = fcBreak
			if i == 2*c.Every-1 {
				k = fcLong
			}
		}
		end := at + c.length(k)
		if el < end || i == 2*c.Every-1 {
			p.idx += i
			p.kind, p.start, p.end, p.session = k, at, end, i/2
			return p
		}
		if k == fcWork {
			p.done++
		}
		at = end
	}
}

// fcToday counts the focus sessions finished today, across runs of the program.
var fcToday struct {
	sync.Mutex
	day string
	n   int
}

func fcAddDone(now time.Time, n int) int {
	fcToday.Lock()
	defer fcToday.Unlock()
	if d := now.Format("2006-01-02"); d != fcToday.day {
		fcToday.day, fcToday.n = d, 0
	}
	fcToday.n += n
	return fcToday.n
}

// Focus is a pomodoro timer: it starts a focus session when the program starts, then runs
// breaks and sessions in turn. The time left is huge in the middle, the phase and its place in
// the cycle on the left, what comes next on the right, and a bar fills along the bottom. The
// screen flashes the new phase's colour when it changes.
type Focus struct {
	cfg func() FocusConfig

	have  bool
	cur   FocusConfig
	t0    float64 // t the current rhythm started (a new config restarts it)
	last  int     // phase index last frame
	done  int     // sessions counted into fcToday
	flash float64 // t of the last phase change
	cache frameCache
}

func NewFocus(cfg func() FocusConfig) *Focus { return &Focus{cfg: cfg, flash: -10} }

func (f *Focus) Frame(t, dt float64, now time.Time) *image.RGBA {
	if c := f.cfg(); !f.have || c != f.cur {
		f.have, f.cur, f.t0, f.last, f.done = true, c, t, 0, 0
	}
	p := fcPhaseAt(f.cur, t-f.t0)
	if p.idx != f.last {
		f.last, f.flash = p.idx, t
	}
	if p.done > f.done {
		fcAddDone(now, p.done-f.done)
		f.done = p.done
	}
	today := fcAddDone(now, 0)
	left := max(0, p.end-(t-f.t0))
	secs := int(left + 0.999)
	img := f.cache.get(fmt.Sprint(p.idx, secs, today, now.Format("15:04")), func(img *image.RGBA) {
		f.render(img, p, secs, today, now, left)
	})
	// the bar along the bottom fills smoothly
	k := (t - f.t0 - p.start) / (p.end - p.start)
	c := p.kind.colour()
	rect(img, 0, H-14, W-1, H-1, mix(bg, c, 0.14))
	rect(img, 0, H-14, int(float64(W-1)*max(0, min(k, 1))), H-1, c)
	if a := t - f.flash; a >= 0 && a < 1.2 {
		fcWash(img, c, 0.45*(1-a/1.2))
	}
	return img
}

// fcWash tints the whole frame towards c.
func fcWash(img *image.RGBA, c color.RGBA, k float64) {
	a := uint32(k * 255)
	for o := 0; o < len(img.Pix); o += 4 {
		img.Pix[o] = blend(a, img.Pix[o], c.R)
		img.Pix[o+1] = blend(a, img.Pix[o+1], c.G)
		img.Pix[o+2] = blend(a, img.Pix[o+2], c.B)
	}
}

const (
	fcX1 = 470  // end of the left column
	fcX2 = 1490 // start of the right column
)

func (f *Focus) render(img *image.RGBA, p fcPhase, secs, today int, now time.Time, left float64) {
	c := p.kind.colour()
	head(img, pad, pad-4, "focus timer", fcX1-pad-30)
	text(img, pad, 70, p.kind.name(), fitPx(p.kind.name(), 64, fcX1-pad-40, 80), c)
	text(img, pad, 170, fmt.Sprintf("SESSION %d OF %d", p.session+1, f.cur.Every), 22, txt2)
	for i := 0; i < f.cur.Every; i++ {
		x := float64(pad+14) + float64(i)*40
		switch {
		case i < p.session || (i == p.session && p.kind != fcWork):
			disc(img, x, 228, 12, amber)
		case i == p.session:
			disc(img, x, 228, 12, mix(bg, amber, 0.35))
			ring(img, int(x), 228, 12, amber)
		default:
			ring(img, int(x), 228, 12, txt3)
		}
	}
	text(img, pad, 276, "TODAY", 22, cyan)
	text(img, float64(pad+110), 270, fmt.Sprintf("%d DONE", today), 30, txt)
	text(img, pad, 330, fmt.Sprintf("%s ON  %s OFF", fcMin(f.cur.Work), fcMin(f.cur.Break)), 20, txt3)
	text(img, pad, 360, fmt.Sprintf("%s LONG BREAK EVERY %d", fcMin(f.cur.Long), f.cur.Every), 20, txt3)
	vrule(img, fcX1-10)

	clock := fmt.Sprintf("%02d:%02d", secs/60, secs%60)
	if secs >= 3600 {
		clock = fmt.Sprintf("%d:%02d:%02d", secs/3600, secs/60%60, secs%60)
	}
	// size on a fixed template so the digits don't jump about as they change
	tmpl := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return '0'
		}
		return r
	}, clock)
	w := fcX2 - fcX1 - 60
	px := fitPx(tmpl, 340, w, H-80)
	b := inkBox(tmpl, px)
	x := float64(fcX1+10) + float64(w-b.Dx())/2 - float64(b.Min.X)
	y := (H-14-b.Dy())/2 - b.Min.Y
	text(img, x, y, clock, px, txt)
	vrule(img, fcX2)

	x2 := float64(fcX2 + 30)
	head(img, int(x2), pad-4, "next", W-pad-int(x2))
	n1 := fcPhaseAt(f.cur, p.end+0.5)
	n2 := fcPhaseAt(f.cur, n1.end+0.5)
	at := now.Add(time.Duration(left * float64(time.Second)))
	for i, n := range []fcPhase{n1, n2} {
		ry := 80 + i*130
		text(img, x2, ry, n.kind.name(), 34, n.kind.colour())
		text(img, x2, ry+48, fmt.Sprintf("AT %s  ·  %s", at.Format("15:04"), fcMin(time.Duration((n.end-n.start)*float64(time.Second)))), 22, txt2)
		at = at.Add(time.Duration((n.end - n.start) * float64(time.Second)))
	}
	text(img, x2, 352, "NOW "+now.Format("15:04"), 22, txt3)
}

func fcMin(d time.Duration) string {
	if d%time.Minute != 0 {
		return fmt.Sprintf("%.1f MIN", d.Minutes())
	}
	return fmt.Sprintf("%d MIN", int(d.Minutes()))
}
