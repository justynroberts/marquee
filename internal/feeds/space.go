package feeds

import (
	"fmt"
	"strings"
	"time"
)

// ISSURL is the Where the ISS at? API for the International Space Station (NORAD 25544).
var ISSURL = "https://api.wheretheiss.at/v1/satellites/25544"

// ISSPoint is where the ISS is (or will be) at one moment.
type ISSPoint struct {
	At         time.Time
	Lat, Lon   float64 // degrees
	Alt        float64 // km
	Speed      float64 // km/h
	Visibility string  // "daylight" (in sunlight) or "eclipsed" (in Earth's shadow)
	Footprint  float64 // diameter of the area that can see it, km
}

type issJSON struct {
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Altitude   float64 `json:"altitude"`
	Velocity   float64 `json:"velocity"`
	Visibility string  `json:"visibility"`
	Footprint  float64 `json:"footprint"`
	Timestamp  int64   `json:"timestamp"`
}

func (j issJSON) point() ISSPoint {
	return ISSPoint{At: time.Unix(j.Timestamp, 0), Lat: j.Latitude, Lon: j.Longitude, Alt: j.Altitude,
		Speed: j.Velocity, Visibility: j.Visibility, Footprint: j.Footprint}
}

// FetchISS returns where the ISS is now.
func FetchISS() (ISSPoint, error) {
	var j issJSON
	if err := getJSON(ISSURL+"?units=kilometers", &j); err != nil {
		return ISSPoint{}, fmt.Errorf("iss: %v", err)
	}
	return j.point(), nil
}

// FetchISSTrack returns n positions step apart from from. The API takes at most ten timestamps
// a call and about one call a second, so longer tracks are fetched in batches with a pause.
func FetchISSTrack(from time.Time, step time.Duration, n int) ([]ISSPoint, error) {
	var out []ISSPoint
	for i := 0; i < n; i += 10 {
		if i > 0 {
			time.Sleep(1100 * time.Millisecond)
		}
		var ts []string
		for k := i; k < n && k < i+10; k++ {
			ts = append(ts, fmt.Sprint(from.Add(time.Duration(k)*step).Unix()))
		}
		var js []issJSON
		if err := getJSON(ISSURL+"/positions?units=kilometers&timestamps="+strings.Join(ts, ","), &js); err != nil {
			return nil, fmt.Errorf("iss track: %v", err)
		}
		for _, j := range js {
			out = append(out, j.point())
		}
	}
	return out, nil
}
