package dashboard

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/justynroberts/marquee/internal/camera"
	"github.com/justynroberts/marquee/internal/feeds"
)

func fakeForecast(string) (*feeds.Forecast, error) {
	f := &feeds.Forecast{Place: "London", Temp: 11.4, Feels: 9, Humidity: 81, Wind: 17, Code: 61, Sunrise: "07:12", Sunset: "16:21", Fetched: time.Now()}
	for i := range 24 {
		f.Hours = append(f.Hours, feeds.HourCast{Hour: (10 + i) % 24, Temp: 11 + 4*math.Sin(float64(i)/4), Rain: (i * 13) % 100, Code: 61})
	}
	for i, d := range []string{"TUE", "WED", "THU", "FRI", "SAT", "SUN"} {
		f.Days = append(f.Days, feeds.DayCast{Day: d, Hi: 12 + float64(i)*4, Lo: 5 + float64(i), Rain: i * 15, Code: []int{61, 3, 0, 80, 95, 2}[i]})
	}
	return f, nil
}

// fakeCamera is a lit disc on a dark gradient.
func fakeCamera(camera.Feature) camera.Frame {
	w, h := 320, 240
	pix := make([]byte, w*h*3)
	for y := range h {
		for x := range w {
			v := byte(x / 8)
			if dx, dy := x-160, y-120; dx*dx+dy*dy < 80*80 {
				v = byte(150 + (x+y)%100)
			}
			pix[(y*w+x)*3], pix[(y*w+x)*3+1], pix[(y*w+x)*3+2] = v, v/2+40, 255-v
		}
	}
	return camera.Frame{W: w, H: h, Pix: pix, Seq: 1}
}

// HIRES_PNG=DIR writes a frame of each visual for eyeballing.
func sampleHeadlines(n int) []feeds.Headline {
	words := []string{"Show HN:", "a", "tiny", "compiler", "for", "LED", "panels", "written", "in", "Go", "that", "renders", "fonts", "exactly", "like", "Pillow", "does"}
	var hs []feeds.Headline
	for i := range n {
		title := ""
		for j := range 4 + (i*7)%13 {
			title += words[(i+j)%len(words)] + " "
		}
		hs = append(hs, feeds.Headline{Source: "HN", Title: title, Points: (i * 97) % 900})
	}
	return hs
}

func TestHiresFrames(t *testing.T) {
	now := time.Date(2026, 12, 31, 23, 59, 7, 0, time.UTC)
	hs := sampleHeadlines(40)
	at := time.Now()
	CameraSource = fakeCamera
	wx := NewWeather(func() string { return "London" })
	wx.fetch = fakeForecast
	wx.Frame(0, 0, now)
	for i := 0; i < 100; i++ { // the fetch runs in the background
		if wx.fc != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	reel := NewNewsreel(func() string { return "hn" })
	reel.fetch = func(feeds.Source) ([]feeds.Headline, error) { return hs, nil }
	reel.Frame(0, 0, now)
	for i := 0; i < 100; i++ {
		reel.mu.Lock()
		got := reel.hs != nil
		reel.mu.Unlock()
		if got {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	fl := NewFlights(func() string { return "London" })
	fl.locate = func(string) (feeds.Place, error) {
		return feeds.Place{Name: "London", Latitude: 51.5, Longitude: -0.12}, nil
	}
	fl.fetch = func(lat, lon float64, nm int) ([]feeds.Aircraft, error) {
		var out []feeds.Aircraft
		for i := range 14 {
			a := feeds.Aircraft{Callsign: fmt.Sprintf("BAW%d", 100+i*7), Type: "A320", Reg: "G-EUP" + string(rune('A'+i)),
				Alt: 3000 + i*2600, Speed: 180 + float64(i*20), Track: float64(i * 47 % 360),
				Lat: lat + 0.4*math.Sin(float64(i)), Lon: lon + 0.6*math.Cos(float64(i*3))}
			if i == 5 {
				a.Emergency, a.Squawk = true, "7700"
			}
			out = append(out, a)
		}
		return out, nil
	}
	qk := NewQuakes()
	qk.fetch = func() ([]feeds.Quake, []feeds.Quake, error) {
		var day, week []feeds.Quake
		for i := range 40 {
			q := feeds.Quake{Mag: 2.5 + float64(i%9)*0.5, Place: fmt.Sprintf("%d km SW of Somewhere, Alaska", i*3),
				Time: now.Add(-time.Duration(i*31) * time.Minute), Lat: 60 - float64(i*3), Lon: -150 + float64(i*9)}
			day = append(day, q)
			q.Mag += 2
			week = append(week, q)
		}
		return day, week, nil
	}
	otd := NewOnThisDay()
	otd.fetch = func(time.Month, int) ([]feeds.Event, error) {
		return []feeds.Event{{Year: 1989, Text: "East German authorities open the Berlin Wall's checkpoints, letting citizens travel to West Berlin for the first time in decades, and crowds begin to take the wall apart."}}, nil
	}
	cd := &Countdown{}
	cd.events, _ = ParseCountdown([]string{"Launch day = 2027-01-01 09:00", "Christmas = 12-25", "Birthday = 03-14"})
	waitFor := func(v interface {
		Frame(t, dt float64, now time.Time) *image.RGBA
	}, ready func() bool) {
		for i := 0; i < 200 && !ready(); i++ {
			v.Frame(0, 0, now)
			time.Sleep(5 * time.Millisecond)
		}
	}
	mk := NewMarkets(func() MarketsConfig { return MarketsConfig{Coins: DefaultCoins, Currency: "usd"} })
	mk.fetch = func(ids []string, cur string) ([]feeds.Coin, error) {
		var out []feeds.Coin
		for i, id := range ids {
			c := feeds.Coin{ID: id, Symbol: strings.ToUpper(id[:3]), Name: id, Price: 73915 / math.Pow(30, float64(i)), Change24h: -1.2 + float64(i)*3.4}
			for h := range 168 {
				c.Week = append(c.Week, c.Price*(1+0.04*math.Sin(float64(h)/14+float64(i))))
			}
			c.High24h, c.Low24h = c.Price*1.02, c.Price*0.97
			out = append(out, c)
		}
		return out, nil
	}
	cal := NewCalendar(func() string { return "england-and-wales" })
	cal.fetch = func(string) ([]feeds.Holiday, error) {
		return []feeds.Holiday{{Title: "Christmas Day", Date: time.Date(2026, 12, 25, 0, 0, 0, 0, time.Local)}, {Title: "Boxing Day", Date: time.Date(2026, 12, 28, 0, 0, 0, 0, time.Local)}, {Title: "New Year's Day", Date: time.Date(2027, 1, 1, 0, 0, 0, 0, time.Local)}}, nil
	}
	aq := NewAirQuality(func() string { return "London" })
	aq.locate = fl.locate
	aq.fetch = func(lat, lon float64) (*feeds.Air, error) {
		a := &feeds.Air{AQI: 43, PM25: 16.6, PM10: 20.6, NO2: 54, O3: 49, Pollen: map[string]float64{"grass": 4, "birch": 62, "alder": 12}, HourStart: 14}
		for h := range 48 {
			a.Hours = append(a.Hours, 40+30*math.Sin(float64(h)/7))
		}
		return a, nil
	}
	trn := NewTrains(func() string { return "LGE" }, func() string { return "tok" })
	trn.fetch = func(tok, st string, win time.Duration) (*feeds.Board, error) {
		b := &feeds.Board{Station: "Long Eaton"}
		dests := []string{"London St Pancras International", "Nottingham", "Derby", "Leicester", "Sheffield", "Crewe", "Norwich"}
		for i, d := range dests {
			c := feeds.TrainCall{Time: now.Add(time.Duration(4+i*9) * time.Minute), Place: d, Platform: fmt.Sprint(1 + i%2), Operator: "East Midlands Railway"}
			switch i {
			case 0:
				c.Delayed = true
			case 2:
				c.Late, c.Expected, c.Reason = 6, c.Time.Add(6*time.Minute), "a points failure"
			case 4:
				c.Cancelled, c.Reason = true, "a shortage of train crew"
			}
			b.Departures = append(b.Departures, c)
			c.Place = dests[len(dests)-1-i]
			b.Arrivals = append(b.Arrivals, c)
		}
		return b, nil
	}
	sun := NewSun(func() string { return "@52.90,-1.27|Long Eaton" })
	grid := NewGridPower(func() string { return "SW1A" })
	grid.fetch = func(string) (*feeds.Grid, error) {
		g := &feeds.Grid{Region: "East Midlands", Intensity: 243, Index: "high",
			Mix: []feeds.Fuel{{Name: "gas", Perc: 56}, {Name: "wind", Perc: 21}, {Name: "nuclear", Perc: 12}, {Name: "solar", Perc: 3}}}
		for i := range 96 {
			g.Forecast = append(g.Forecast, feeds.HalfHour{From: now.Add(time.Duration(i) * 30 * time.Minute), Intensity: 120 + int(120*math.Sin(float64(i)/9))})
		}
		return g, nil
	}
	aur := NewAurora()
	aur.fetch = func() (*feeds.Aurora, error) {
		a := &feeds.Aurora{Status: "amber"}
		for i := range 24 {
			a.Activity = append(a.Activity, feeds.AuroraHour{At: now.Add(time.Duration(i-24) * time.Hour), Value: float64(20 + i*6), Status: "green"})
		}
		for i := range 56 {
			a.Kp = append(a.Kp, feeds.KpPoint{At: now.Add(time.Duration(i-32) * 3 * time.Hour), Kp: 2 + 3*math.Sin(float64(i)/5), Predicted: i > 32})
		}
		return a, nil
	}
	waitFor(sun, func() bool { sun.mu.Lock(); defer sun.mu.Unlock(); return sun.home != nil })
	waitFor(grid, func() bool { grid.poll.mu.Lock(); defer grid.poll.mu.Unlock(); return grid.poll.ok })
	waitFor(aur, func() bool { aur.poll.mu.Lock(); defer aur.poll.mu.Unlock(); return aur.poll.ok })
	waitFor(trn, func() bool { trn.poll.mu.Lock(); defer trn.poll.mu.Unlock(); return trn.poll.ok })
	waitFor(mk, func() bool { mk.poll.mu.Lock(); defer mk.poll.mu.Unlock(); return mk.poll.ok })
	waitFor(cal, func() bool { cal.poll.mu.Lock(); defer cal.poll.mu.Unlock(); return cal.poll.ok })
	waitFor(aq, func() bool { aq.poll.mu.Lock(); defer aq.poll.mu.Unlock(); return aq.poll.ok })
	waitFor(fl, func() bool { fl.poll.mu.Lock(); defer fl.poll.mu.Unlock(); return fl.poll.ok })
	waitFor(qk, func() bool { qk.poll.mu.Lock(); defer qk.poll.mu.Unlock(); return qk.poll.ok })
	waitFor(otd, func() bool { otd.poll.mu.Lock(); defer otd.poll.mu.Unlock(); return otd.poll.ok })
	visuals := map[string]interface {
		Frame(t, dt float64, now time.Time) *image.RGBA
	}{
		"bigclock":   NewBigClock(),
		"worldclock": NewWorldClock(nil),
		"headlines":  NewHeadlines(func() ([]feeds.Headline, time.Time) { return hs, at }),
		"weather":    wx,
		"ascii":      NewASCII(),
		"newsreel":   reel,
		"flights":    fl,
		"quakes":     qk,
		"onthisday":  otd,
		"moon":       NewMoon(),
		"countdown":  cd,
		"markets":    mk,
		"calendar":   cal,
		"air":        aq,
		"wordclock":  NewWordClock(),
		"trains":     trn,
		"sun":        sun,
		"grid":       grid,
		"aurora":     aur,
		"warroom":    NewWarRoom(),
	}
	for name, v := range visuals {
		img := v.Frame(0, 0, now)
		if img == nil || img.Bounds().Dx() != W || img.Bounds().Dy() != H {
			t.Fatalf("%s: bad frame", name)
		}
		lit := 0
		for i := 0; i < len(img.Pix); i += 4 {
			if max(img.Pix[i], img.Pix[i+1], img.Pix[i+2]) > 100 {
				lit++
			}
		}
		if lit < 2000 {
			t.Errorf("%s: only %d lit pixels", name, lit)
		}
		// later frames must stay within the frame budget once warm
		start := time.Now()
		for i := 1; i <= 10; i++ {
			v.Frame(float64(i)/12, 1.0/12, now.Add(time.Duration(i)*time.Second))
		}
		if per := time.Since(start) / 10; per > 40*time.Millisecond {
			t.Errorf("%s: %v per frame", name, per)
		}
		if dir := os.Getenv("HIRES_PNG"); dir != "" {
			f, _ := os.Create(filepath.Join(dir, name+".png"))
			png.Encode(f, v.Frame(3, 0, now))
			f.Close()
		}
	}
}

func TestParseZones(t *testing.T) {
	z, bad := ParseZones([]string{"# comment", "", "Home = Europe/London", "America/New_York", "Mars = Olympus/Mons"})
	if len(z) != 2 || z[0].Label != "Home" || z[1].Label != "New York" || len(bad) != 1 {
		t.Fatalf("%+v %v", z, bad)
	}
}

func TestDayNumberAcrossYears(t *testing.T) {
	a := time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC)
	if d := dayNumber(a.Add(2*time.Hour)) - dayNumber(a); d != 1 {
		t.Fatal(d)
	}
}

func TestWrap(t *testing.T) {
	lines := wrap("one two three four five six seven eight nine ten eleven twelve", 23, 120, 2)
	if len(lines) != 2 {
		t.Fatalf("%q", lines)
	}
	for _, l := range lines {
		if measure(l, 23) > 120 {
			t.Errorf("too wide: %q", l)
		}
	}
	if fmt.Sprint(wrap("short", 23, 400, 3)) != "[short]" {
		t.Error("short")
	}
}

func TestNewsreelRows(t *testing.T) {
	r := NewNewsreel(func() string { return "nope" })
	r.Frame(0, 0, time.Now())
	if r.err == nil || !strings.Contains(r.err.Error(), "bbc") {
		t.Fatalf("unknown source error = %v", r.err)
	}
	r.load(sampleHeadlines(5))
	if len(r.rows[0].items) != 3 || len(r.rows[1].items) != 2 {
		t.Fatalf("rows %d/%d", len(r.rows[0].items), len(r.rows[1].items))
	}
	// a second load waits until the row comes round
	r.load(sampleHeadlines(9))
	row := &r.rows[0]
	if row.pending == nil || len(row.items) != 3 {
		t.Fatal("reload applied mid-row")
	}
	for i := 0; i < 100000 && row.pending != nil; i++ {
		row.advance()
	}
	if len(row.items) != 5 || row.pos != 0 {
		t.Fatalf("after wrap: %d items, pos %d", len(row.items), row.pos)
	}
	if got := shortLabel("Ars Technica - All content"); got != "Ars Technica" {
		t.Errorf("shortLabel = %q", got)
	}
}

func TestParseCountdown(t *testing.T) {
	evs, bad := ParseCountdown([]string{"# x", "Xmas = 12-25", "Trip = 2027-03-01 08:30", "nope", "Bad = 31-31"})
	if len(evs) != 2 || len(bad) != 2 {
		t.Fatalf("evs %v bad %v", evs, bad)
	}
	now := time.Date(2026, 12, 26, 10, 0, 0, 0, time.Local)
	if got := evs[0].next(now); got.Year() != 2027 || daysUntil(now, got) != 364 {
		t.Errorf("Xmas after the day: %v (%d days)", got, daysUntil(now, got))
	}
	if got := evs[0].next(time.Date(2026, 12, 25, 18, 0, 0, 0, time.Local)); got.Year() != 2026 {
		t.Errorf("Xmas on the day rolled over: %v", got)
	}
}

func TestMoonPhase(t *testing.T) {
	// full moon 2026-11-24 14:53 UTC, new moon 2026-12-09 00:52 UTC
	full := MoonAge(time.Date(2026, 11, 24, 15, 0, 0, 0, time.UTC))
	if MoonPhase(full) != "Full moon" || MoonIllumination(full) < 0.99 {
		t.Errorf("full: age %.2f %s %.2f", full, MoonPhase(full), MoonIllumination(full))
	}
	nw := MoonAge(time.Date(2026, 12, 9, 1, 0, 0, 0, time.UTC))
	if MoonPhase(nw) != "New moon" || MoonIllumination(nw) > 0.01 {
		t.Errorf("new: age %.2f %s", nw, MoonPhase(nw))
	}
}

func TestWordClock(t *testing.T) {
	for i, row := range wcRows {
		n := 0
		for _, tk := range row {
			n += len(tk.text)
		}
		if n != wcCols {
			t.Errorf("row %d has %d letters", i, n)
		}
	}
	// every phrase must read in grid order: walk the tokens and tick off lit words in turn
	for m := 0; m < 24*60; m += 5 {
		at := time.Date(2026, 1, 1, m/60, m%60, 0, 0, time.UTC)
		lit := WordsFor(at)
		seen := 0
		for _, row := range wcRows {
			for _, tk := range row {
				if tk.key != "" && lit[tk.key] {
					seen++
				}
			}
		}
		if seen != len(lit) {
			t.Fatalf("%s: %d of %d words found in the grid", at.Format("15:04"), seen, len(lit))
		}
	}
	got := WordsFor(time.Date(2026, 1, 1, 22, 37, 0, 0, time.UTC))
	for _, k := range []string{"twenty", "m5", "minutes", "to", "h11", "at", "night"} {
		if !got[k] {
			t.Errorf("22:37 is missing %s: %v", k, got)
		}
	}
}

func TestLoadMarkets(t *testing.T) {
	p := t.TempDir() + "/markets.txt"
	os.WriteFile(p, []byte("# coins\nBitcoin\nshiba inu\ncurrency = GBP\n"), 0o644)
	c := LoadMarkets(p)
	if c.Currency != "gbp" || len(c.Coins) != 2 || c.Coins[1] != "shiba-inu" {
		t.Fatalf("%+v", c)
	}
	if money(73915.4) != "73,915" || money(0.000123) != "0.0001230" {
		t.Errorf("money: %s %s", money(73915.4), money(0.000123))
	}
}

func TestSunTimes(t *testing.T) {
	// Long Eaton, 10 November 2026: sunrise about 07:19, sunset about 16:18 (GMT)
	d := SunTimes(time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC), 52.90, -1.27)
	for _, c := range []struct {
		name string
		got  time.Time
		h, m int
	}{{"sunrise", d.Rise, 7, 19}, {"sunset", d.Set, 16, 18}} {
		want := time.Date(2026, 11, 10, c.h, c.m, 0, 0, time.UTC)
		if diff := c.got.Sub(want); diff < -3*time.Minute || diff > 3*time.Minute {
			t.Errorf("%s %s, want about %s", c.name, c.got.Format("15:04"), want.Format("15:04"))
		}
	}
}
