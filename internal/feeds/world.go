package feeds

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Endpoints for the world screens (tests point them elsewhere). None needs a key.
var (
	AircraftURL  = "https://api.adsb.lol/v2/point" // /{lat}/{lon}/{radius nm}
	QuakesDayURL = "https://earthquake.usgs.gov/earthquakes/feed/v1.0/summary/2.5_day.geojson"
	QuakesMonURL = "https://earthquake.usgs.gov/earthquakes/feed/v1.0/summary/4.5_month.geojson"
	OnThisDayURL = "https://api.wikimedia.org/feed/v1/wikipedia/en/onthisday/events" // /MM/DD
)

// ------------------------------------------------------------------ aircraft

// Aircraft is one plane seen by the ADS-B Exchange-compatible adsb.lol network.
type Aircraft struct {
	Callsign  string
	Reg, Type string
	Alt       int // feet; 0 on the ground
	Ground    bool
	Speed     float64 // knots over the ground
	Track     float64 // degrees
	Lat, Lon  float64
	Dist      float64 // nautical miles from the asked point
	Squawk    string
	Emergency bool
}

// FetchAircraft returns the aircraft within radius nautical miles of lat, lon, nearest first.
func FetchAircraft(lat, lon float64, radius int) ([]Aircraft, error) {
	var resp struct {
		AC []struct {
			Flight    string          `json:"flight"`
			Reg       string          `json:"r"`
			Type      string          `json:"t"`
			Alt       json.RawMessage `json:"alt_baro"` // a number, or "ground"
			GS        float64         `json:"gs"`
			Track     float64         `json:"track"`
			Lat       *float64        `json:"lat"`
			Lon       *float64        `json:"lon"`
			Dst       float64         `json:"dst"`
			Squawk    string          `json:"squawk"`
			Emergency string          `json:"emergency"`
		} `json:"ac"`
	}
	if err := getJSON(fmt.Sprintf("%s/%.3f/%.3f/%d", AircraftURL, lat, lon, radius), &resp); err != nil {
		return nil, err
	}
	var out []Aircraft
	for _, a := range resp.AC {
		if a.Lat == nil || a.Lon == nil {
			continue
		}
		ac := Aircraft{
			Callsign: strings.TrimSpace(a.Flight), Reg: a.Reg, Type: a.Type, Speed: a.GS, Track: a.Track,
			Lat: *a.Lat, Lon: *a.Lon, Dist: a.Dst, Squawk: a.Squawk,
			Emergency: (a.Emergency != "" && a.Emergency != "none") || a.Squawk == "7500" || a.Squawk == "7600" || a.Squawk == "7700",
		}
		if string(a.Alt) == `"ground"` {
			ac.Ground = true
		} else {
			_ = json.Unmarshal(a.Alt, &ac.Alt)
		}
		if ac.Callsign == "" {
			ac.Callsign = ac.Reg
		}
		out = append(out, ac)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dist < out[j].Dist })
	return out, nil
}

// ------------------------------------------------------------------ earthquakes

// Quake is one earthquake from the USGS feeds.
type Quake struct {
	Mag      float64
	Place    string
	Time     time.Time
	Lat, Lon float64
	Depth    float64 // km
}

// FetchQuakes returns the last day's quakes of magnitude 2.5+ and the last 30 days' of 4.5+,
// each newest first.
func FetchQuakes() (day, month []Quake, err error) {
	if day, err = fetchQuakes(QuakesDayURL); err != nil {
		return nil, nil, err
	}
	month, _ = fetchQuakes(QuakesMonURL) // only the backdrop; the day's list stands alone
	return day, month, nil
}

func fetchQuakes(u string) ([]Quake, error) {
	var resp struct {
		Features []struct {
			Properties struct {
				Mag   *float64 `json:"mag"`
				Place string   `json:"place"`
				Time  int64    `json:"time"`
			} `json:"properties"`
			Geometry struct {
				Coordinates []float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := getJSON(u, &resp); err != nil {
		return nil, err
	}
	var out []Quake
	for _, f := range resp.Features {
		c := f.Geometry.Coordinates
		if f.Properties.Mag == nil || len(c) < 3 {
			continue
		}
		out = append(out, Quake{Mag: *f.Properties.Mag, Place: f.Properties.Place, Time: time.UnixMilli(f.Properties.Time),
			Lon: c[0], Lat: c[1], Depth: c[2]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out, nil
}

// ------------------------------------------------------------------ on this day

// Event is one Wikipedia "on this day" event.
type Event struct {
	Year int
	Text string
}

// FetchOnThisDay returns the events Wikipedia lists for a calendar day, oldest first.
func FetchOnThisDay(month time.Month, day int) ([]Event, error) {
	var resp struct {
		Events []struct {
			Year int    `json:"year"`
			Text string `json:"text"`
		} `json:"events"`
	}
	if err := getJSON(fmt.Sprintf("%s/%02d/%02d", OnThisDayURL, int(month), day), &resp); err != nil {
		return nil, err
	}
	var out []Event
	for _, e := range resp.Events {
		if t := strings.Join(strings.Fields(e.Text), " "); t != "" {
			out = append(out, Event{e.Year, t})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("on this day: no events")
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Year < out[j].Year })
	return out, nil
}
