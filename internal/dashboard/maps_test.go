package dashboard

import (
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

func mapsLit(img *image.RGBA) int {
	n := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if max(img.Pix[i], img.Pix[i+1], img.Pix[i+2]) > 60 {
			n++
		}
	}
	return n
}

func mapsSave(t *testing.T, name string, img *image.RGBA) {
	dir := os.Getenv("HIRES_PNG")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, img)
}

func mapsWait(t *testing.T, ok func() bool) {
	for i := 0; i < 200 && !ok(); i++ {
		time.Sleep(10 * time.Millisecond)
	}
}

var mapsHome = feeds.Place{Name: "Westminster", Latitude: 51.50, Longitude: -0.14}

func TestMapsISS(t *testing.T) {
	now := time.Now()
	s := NewISS(func() string { return "home" })
	s.home.locate = func(string) (feeds.Place, error) { return mapsHome, nil }
	// a made-up orbit: 51.6° inclined, 92 minutes round
	orbit := func(at time.Time) feeds.ISSPoint {
		u := float64(at.Unix()%5520) / 5520 * 2 * math.Pi
		lat := math.Asin(math.Sin(51.6*math.Pi/180)*math.Sin(u)) * 180 / math.Pi
		lon := dnLon(math.Atan2(math.Cos(51.6*math.Pi/180)*math.Sin(u), math.Cos(u))*180/math.Pi - float64(at.Unix()%86400)/240)
		return feeds.ISSPoint{At: at, Lat: lat, Lon: lon, Alt: 420, Speed: 27600, Visibility: "daylight", Footprint: 4500}
	}
	s.fetchNow = func() (feeds.ISSPoint, error) { return orbit(now), nil }
	s.fetchTrack = func(from time.Time, step time.Duration, n int) ([]feeds.ISSPoint, error) {
		var out []feeds.ISSPoint
		for i := range n {
			out = append(out, orbit(from.Add(time.Duration(i)*step)))
		}
		return out, nil
	}
	s.Frame(0, 0, now)
	mapsWait(t, func() bool {
		_, _, a, _ := s.pos.get("", nil)
		_, _, b, _ := s.track.get("", nil)
		h, _ := s.home.where()
		return a && b && h != nil
	})
	img := s.Frame(1, 0, now)
	if mapsLit(img) < 20000 {
		t.Fatalf("ISS frame nearly empty: %d lit", mapsLit(img))
	}
	mapsSave(t, "iss", img)
	// the dot follows the track between its points
	lat, lon, ok := issAt([]feeds.ISSPoint{{At: now, Lat: 0, Lon: 0}, {At: now.Add(time.Minute), Lat: 0, Lon: 10}}, now.Add(30*time.Second))
	if !ok || math.Abs(lat) > 1e-6 || math.Abs(lon-5) > 1e-6 {
		t.Fatalf("issAt = %v %v %v", lat, lon, ok)
	}
	if e := issElevation(0, 420); math.Abs(e-90) > 1e-6 {
		t.Fatalf("overhead elevation %v", e)
	}
}

func TestMapsDayNight(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	lat, lon := SubSolar(now)
	if math.Abs(lat-23.44) > 0.3 || math.Abs(lon) > 3 {
		t.Fatalf("subsolar at June solstice noon UTC = %.2f, %.2f", lat, lon)
	}
	if lat, _ := SubSolar(time.Date(2026, 12, 21, 12, 0, 0, 0, time.UTC)); math.Abs(lat+23.44) > 0.3 {
		t.Fatalf("December subsolar latitude %.2f", lat)
	}
	if ml, _ := SubLunar(now); math.Abs(ml) > 29 {
		t.Fatalf("sublunar latitude %.2f", ml)
	}
	z, _ := ParseZones(DefaultZones)
	d := NewDayNight(func() string { return "home" }, func() []Zone { return z })
	d.home.locate = func(string) (feeds.Place, error) { return mapsHome, nil }
	d.Frame(0, 0, now)
	mapsWait(t, func() bool { h, _ := d.home.where(); return h != nil })
	img := d.Frame(0, 0, time.Date(2026, 11, 10, 15, 30, 0, 0, time.UTC))
	if mapsLit(img) < 20000 {
		t.Fatalf("day/night frame nearly empty: %d lit", mapsLit(img))
	}
	mapsSave(t, "daynight", img)
}

func TestMapsOrrery(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	earth := orrPlanets[orrEarth].Position(now)
	if r := math.Hypot(earth[0], earth[1]); math.Abs(r-1) > 0.02 {
		t.Fatalf("Earth %.3f AU from the sun", r)
	}
	mars := orrPlanets[3].Position(now)
	if d := orrDist(earth, mars); d < 0.37 || d > 2.7 {
		t.Fatalf("Mars %.2f AU from Earth", d)
	}
	// Earth is at ecliptic longitude ~ sun's + 180: in November the sun is in Scorpius/Libra
	// (~228°), so Earth is near 48°.
	if l := math.Atan2(earth[1], earth[0]) * 180 / math.Pi; math.Abs(l-48) > 3 {
		t.Fatalf("Earth heliocentric longitude %.1f", l)
	}
	o := NewOrrery()
	img := o.Frame(3, 0, now)
	if mapsLit(img) < 8000 {
		t.Fatalf("orrery frame nearly empty: %d lit", mapsLit(img))
	}
	mapsSave(t, "orrery", img)
}
