package feeds

import (
	"fmt"
	"time"
)

// RainCast is the rain expected at a place: every 15 minutes for the next two hours, then
// hourly for twelve, from Open-Meteo, in the place's own time zone.
type RainCast struct {
	Place    string
	Quarters []RainSlot // 15-minute slots from the current one, 8 of them
	Hours    []RainSlot // hourly slots from the current hour, 12 of them
}

// RainSlot is the rain in one slot starting At: millimetres and the chance of any, in percent.
type RainSlot struct {
	At     time.Time
	MM     float64
	Chance int
}

// FetchRain looks a place up and fetches its rain for the next hours.
func FetchRain(location string) (*RainCast, error) {
	p, err := Geocode(location)
	if err != nil {
		return nil, err
	}
	var r struct {
		Offset int `json:"utc_offset_seconds"`
		Min    struct {
			Time   []string  `json:"time"`
			MM     []float64 `json:"precipitation"`
			Chance []*int    `json:"precipitation_probability"`
		} `json:"minutely_15"`
		Hourly struct {
			Time   []string  `json:"time"`
			MM     []float64 `json:"precipitation"`
			Chance []*int    `json:"precipitation_probability"`
		} `json:"hourly"`
	}
	u := fmt.Sprintf("%s?latitude=%g&longitude=%g&timezone=auto&minutely_15=precipitation,precipitation_probability"+
		"&hourly=precipitation,precipitation_probability&past_minutely_15=1&forecast_minutely_15=10&forecast_hours=13",
		ForecastURL, p.Latitude, p.Longitude)
	if err := getJSON(u, &r); err != nil {
		return nil, err
	}
	zone := time.FixedZone("", r.Offset)
	slots := func(ts []string, mm []float64, ch []*int, step time.Duration, n int) []RainSlot {
		now := time.Now()
		var out []RainSlot
		for i, s := range ts {
			at, err := time.ParseInLocation("2006-01-02T15:04", s, zone)
			if err != nil || i >= len(mm) || !at.Add(step).After(now) {
				continue
			}
			sl := RainSlot{At: at, MM: mm[i]}
			if i < len(ch) && ch[i] != nil {
				sl.Chance = *ch[i]
			}
			out = append(out, sl)
			if len(out) == n {
				break
			}
		}
		return out
	}
	rc := &RainCast{Place: p.Name,
		Quarters: slots(r.Min.Time, r.Min.MM, r.Min.Chance, 15*time.Minute, 8),
		Hours:    slots(r.Hourly.Time, r.Hourly.MM, r.Hourly.Chance, time.Hour, 12)}
	if len(rc.Quarters) == 0 && len(rc.Hours) == 0 {
		return nil, fmt.Errorf("rain: no forecast for %s", p.Name)
	}
	return rc, nil
}
