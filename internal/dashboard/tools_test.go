package dashboard

import (
	"errors"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

func toolsLit(img *image.RGBA) int {
	n := 0
	for o := 0; o < len(img.Pix); o += 4 {
		if max(img.Pix[o], img.Pix[o+1], img.Pix[o+2]) > 80 {
			n++
		}
	}
	return n
}

func toolsSave(t *testing.T, name string, img *image.RGBA) {
	if dir := os.Getenv("HIRES_PNG"); dir != "" {
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		png.Encode(f, img)
		f.Close()
	}
}

func TestProgressMaths(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 11, 10, 18, 30, 0, 0, loc) // a Tuesday
	if ws := pgWeekStart(now); !ws.Equal(time.Date(2026, 11, 9, 0, 0, 0, 0, loc)) {
		t.Errorf("week start %v", ws)
	}
	if ws := pgWeekStart(time.Date(2026, 11, 15, 23, 0, 0, 0, loc)); ws.Day() != 9 { // Sunday
		t.Errorf("sunday's week starts %v", ws)
	}
	if w := pgISOWeek(time.Date(2027, 1, 1, 12, 0, 0, 0, loc)); w != 53 {
		t.Errorf("1 Jan 2027 is ISO week %d, want 53", w)
	}
	sp := pgSpans(now)
	if f := pgFrac(now, sp[1].from, sp[1].to); math.Abs(f-18.5/24) > 1e-9 {
		t.Errorf("today %v", f)
	}
	if sp[4].label != "Q4" || sp[7].label != "CENTURY 21ST" || sp[6].label != "THE 2020S" {
		t.Errorf("labels %q %q %q", sp[4].label, sp[6].label, sp[7].label)
	}
	if n := pgWorkdaysLeft(now); n != 15 { // 10-30 Nov 2026
		t.Errorf("workdays left %d", n)
	}
	if pgDaysInYear(2028) != 366 || pgDaysInYear(2026) != 365 {
		t.Error("leap years")
	}
	if pgOrdinal(11) != "11TH" || pgOrdinal(22) != "22ND" {
		t.Error("ordinals")
	}
}

func TestParseFocus(t *testing.T) {
	ok := map[string]FocusConfig{
		"25 5":               {25 * time.Minute, 5 * time.Minute, 15 * time.Minute, 4},
		"25/5":               {25 * time.Minute, 5 * time.Minute, 15 * time.Minute, 4},
		"50 10 30 4":         {50 * time.Minute, 10 * time.Minute, 30 * time.Minute, 4},
		"25m 5m":             {25 * time.Minute, 5 * time.Minute, 15 * time.Minute, 4},
		"45 10":              {45 * time.Minute, 10 * time.Minute, 30 * time.Minute, 4},
		"25min, 5min, 20, 3": {25 * time.Minute, 5 * time.Minute, 20 * time.Minute, 3},
	}
	for in, want := range ok {
		got, err := ParseFocus(in)
		if err != nil || got != want {
			t.Errorf("%q: %+v %v", in, got, err)
		}
	}
	for _, in := range []string{"", "25", "0 5", "25 0", "300 5", "25 5 15 0", "abc 5", "1 2 3 4 5"} {
		if _, err := ParseFocus(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "focus.txt")
	os.WriteFile(p, []byte("# minutes\n\n50 10\n"), 0o644)
	if c := LoadFocus(p); c.Work != 50*time.Minute {
		t.Errorf("load %+v", c)
	}
	if c := LoadFocus(filepath.Join(dir, "none")); c != DefaultFocus {
		t.Errorf("default %+v", c)
	}
}

func TestFocusPhases(t *testing.T) {
	c := FocusConfig{Work: 10 * time.Minute, Break: 2 * time.Minute, Long: 5 * time.Minute, Every: 3}
	want := []fcKind{fcWork, fcBreak, fcWork, fcBreak, fcWork, fcLong, fcWork, fcBreak}
	el := 0.0
	for i, k := range want {
		p := fcPhaseAt(c, el+1)
		if p.kind != k || p.idx != i || p.start != el {
			t.Fatalf("phase %d: %+v", i, p)
		}
		if p.done != (i+1)/2 { // a session finishes at each break
			t.Errorf("phase %d: done %d", i, p.done)
		}
		el = p.end
	}
	if p := fcPhaseAt(c, 0); p.session != 0 {
		t.Errorf("first session %+v", p)
	}
}

func TestToolsFrames(t *testing.T) {
	now := time.Date(2026, 11, 10, 14, 7, 33, 0, time.UTC)

	pg := NewProgress()
	img := pg.Frame(0, 0, now)
	if n := toolsLit(img); n < 20000 {
		t.Errorf("progress: %d lit", n)
	}
	toolsSave(t, "progress", img)

	fc := NewFocus(func() FocusConfig { return DefaultFocus })
	fc.Frame(0, 0, now)
	img = fc.Frame(9*60+12, 0, now)
	if n := toolsLit(img); n < 20000 {
		t.Errorf("focus: %d lit", n)
	}
	toolsSave(t, "focus", img)
	img = fc.Frame(25*60+0.3, 0, now.Add(25*time.Minute)) // just into the break: flashing
	toolsSave(t, "focus-break", img)
	if fcAddDone(now, 0) < 1 {
		t.Error("the finished session was not counted")
	}

	nw := NewNetWatch()
	nw.gateway = func() string { return "192.168.1.1" }
	calls := 0
	nw.probe = func(kind, addr string) (time.Duration, error) {
		calls++
		switch kind {
		case "gw":
			return time.Duration(2+calls%3) * time.Millisecond, nil
		case "dns":
			return time.Duration(10+calls%40) * time.Millisecond, nil
		}
		if calls%37 == 0 {
			return 0, errors.New("timeout")
		}
		return time.Duration(14+15*math.Sin(float64(calls)/9)+float64(calls%7)) * time.Millisecond, nil
	}
	start := now.Add(-nwWindow)
	for i := 0; i < 150; i++ {
		nw.round(start.Add(time.Duration(i) * nwEvery))
	}
	nw.mu.Lock()
	nw.running = true // keep the real loop out of the test
	nw.mu.Unlock()
	img = nw.Frame(0, 0, now)
	if n := toolsLit(img); n < 20000 {
		t.Errorf("netwatch: %d lit", n)
	}
	if w, _, _ := nw.verdict(); w != "ONLINE" {
		t.Errorf("verdict %s", w)
	}
	toolsSave(t, "netwatch", img)

	// everything beyond the router stops answering
	nw.probe = func(kind, addr string) (time.Duration, error) {
		if kind == "gw" {
			return 3 * time.Millisecond, nil
		}
		return 0, errors.New("timeout")
	}
	nw.round(now.Add(2 * time.Second))
	nw.round(now.Add(4 * time.Second))
	if w, _, _ := nw.verdict(); w != "DOWN" {
		t.Errorf("verdict when down: %s", w)
	}
	nw.mu.Lock()
	nw.cache.key = ""
	nw.mu.Unlock()
	toolsSave(t, "netwatch-down", nw.Frame(0, 0, now.Add(4*time.Second)))
	if s := nw.targets[0].stats(); s.loss <= 0 || s.hi < s.avg || s.lo > s.avg {
		t.Errorf("stats %+v", s)
	}
	if nwNice(23) != 50 || nwNice(101) != 200 || nwNice(9) != 10 {
		t.Error("nice scale")
	}
}

func TestClaudeCodeFrame(t *testing.T) {
	now := time.Date(2026, 11, 11, 15, 20, 0, 0, time.Local)
	u := &feeds.ClaudeUsage{Today: feeds.ClaudeTokens{In: 4200, Out: 812_000, CacheWrite: 2_100_000, CacheRead: 96_000_000, Calls: 1830}}
	for h := 8; h <= 15; h++ {
		u.Hours[h] = feeds.ClaudeTokens{Out: int64(40_000 + h*9000)}
	}
	for i := 0; i < 7; i++ {
		u.Days = append(u.Days, feeds.ClaudeDay{Day: now.AddDate(0, 0, i-6), ClaudeTokens: feeds.ClaudeTokens{Out: int64(300_000 + i*90_000)}})
	}
	u.Sessions = []feeds.ClaudeSession{{Name: "marquee", Busy: true, Since: now.Add(-3 * time.Minute), Context: 183_000, Today: feeds.ClaudeTokens{Out: 410_000}},
		{Name: "rookery", Since: now.Add(-40 * time.Minute), Context: 64_000, Today: feeds.ClaudeTokens{Out: 120_000}}}
	u.Projects = []feeds.Named{{Name: "marquee", ClaudeTokens: feeds.ClaudeTokens{Out: 410_000}}, {Name: "rookery", ClaudeTokens: feeds.ClaudeTokens{Out: 120_000}}}
	c := &ClaudeCode{scan: func(time.Time) (*feeds.ClaudeUsage, error) { return u, nil }, poll: poller[*feeds.ClaudeUsage]{every: time.Hour}}
	var img *image.RGBA
	for i := 0; i < 100; i++ {
		if img = c.Frame(float64(i), 0, now); len(c.busy) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	lit := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if max(img.Pix[i], img.Pix[i+1], img.Pix[i+2]) > 100 {
			lit++
		}
	}
	if lit < 2000 || len(c.busy) != 1 {
		t.Fatalf("lit %d, busy %d", lit, len(c.busy))
	}
	if dir := os.Getenv("HIRES_PNG"); dir != "" {
		f, _ := os.Create(filepath.Join(dir, "claude.png"))
		png.Encode(f, img)
		f.Close()
	}
}

func fakeClaudeUsage(now time.Time) *feeds.ClaudeUsage {
	u := &feeds.ClaudeUsage{Today: feeds.ClaudeTokens{In: 4200, Out: 812_000, CacheWrite: 2_100_000, CacheRead: 96_000_000, Calls: 1830},
		ProjectHours: map[string][24]int64{}}
	for d := 0; d < 7; d++ {
		u.Days = append(u.Days, feeds.ClaudeDay{Day: now.AddDate(0, 0, d-6), ClaudeTokens: feeds.ClaudeTokens{Out: int64(300_000 + d*90_000)}})
		for h := 7; h < 23; h++ {
			if d == 6 && h > now.Hour() {
				break
			}
			u.Week[d][h] = int64((h*7919+d*104729)%60_000 + 2000)
		}
	}
	for i, p := range []string{"marquee", "rookery", "mmo-engine", "finscreen"} {
		var hrs [24]int64
		for h := 8; h <= now.Hour(); h++ {
			hrs[h] = int64(((h+i*5)%7 + 1) * (12000 - i*2000))
		}
		u.ProjectHours[p] = hrs
		u.Hours[8+i] = feeds.ClaudeTokens{Out: int64(40_000 + i*9000)}
	}
	u.Sessions = []feeds.ClaudeSession{{Name: "marquee", Busy: true, Since: now.Add(-3 * time.Minute), Context: 183_000, Today: feeds.ClaudeTokens{Out: 410_000}},
		{Name: "rookery", Since: now.Add(-40 * time.Minute), Context: 64_000, Today: feeds.ClaudeTokens{Out: 120_000}},
		{Name: "mmo-engine", Busy: true, Context: 640_000, Today: feeds.ClaudeTokens{Out: 220_000}}}
	return u
}

func TestClaudeFunFrames(t *testing.T) {
	now := time.Date(2026, 11, 11, 15, 20, 0, 0, time.Local)
	u := fakeClaudeUsage(now)
	fake := func() *ccLive {
		l := &ccLive{poll: poller[*feeds.ClaudeUsage]{every: time.Hour}, sessRate: map[string]float64{}, rate: 4000}
		l.scan = func(time.Time) (*feeds.ClaudeUsage, error) { return u, nil }
		return l
	}
	for i := 0; i < 40; i++ {
		u.Recent = append(u.Recent, feeds.ClaudeCall{At: now.Add(time.Duration(i-40) * time.Minute), Out: int64(200 + i*i*40), Project: []string{"marquee", "rookery", "mmo-engine"}[i%3]})
	}
	hive, city, flow := NewClaudeHive(), NewClaudeCity(), NewClaudeFlow()
	clock, orbit, burst, meter := NewClaudeClock(), NewClaudeOrbit(), NewClaudeBurst(), NewClaudeMeter()
	hive.live, city.live, flow.live = fake(), fake(), fake()
	clock.live, orbit.live, burst.live, meter.live = fake(), fake(), fake(), fake()
	lives := map[string]*ccLive{"claudehive": hive.live, "claudecity": city.live, "claudeflow": flow.live,
		"claudeclock": clock.live, "claudeorbit": orbit.live, "claudeburst": burst.live, "claudemeter": meter.live}
	for name, v := range map[string]interface {
		Frame(t, dt float64, now time.Time) *image.RGBA
	}{"claudehive": hive, "claudecity": city, "claudeflow": flow, "claudeclock": clock, "claudeorbit": orbit, "claudeburst": burst, "claudemeter": meter} {
		var img *image.RGBA
		for i := 0; i < 100; i++ {
			img = v.Frame(float64(i)/12, 1.0/12, now)
			if _, _, ok, _ := lives[name].get(); ok && i > 40 {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		lit := 0
		for i := 0; i < len(img.Pix); i += 4 {
			if max(img.Pix[i], img.Pix[i+1], img.Pix[i+2]) > 100 {
				lit++
			}
		}
		if lit < 3000 {
			t.Errorf("%s: only %d lit pixels", name, lit)
		}
		start := time.Now()
		for i := 0; i < 10; i++ {
			v.Frame(5+float64(i)/12, 1.0/12, now)
		}
		if per := time.Since(start) / 10; per > 40*time.Millisecond {
			t.Errorf("%s: %v per frame", name, per)
		}
		if dir := os.Getenv("HIRES_PNG"); dir != "" {
			f, _ := os.Create(filepath.Join(dir, name+".png"))
			png.Encode(f, img)
			f.Close()
		}
	}
	if len(hive.spark) == 0 {
		t.Error("no sparks from working sessions")
	}
}
