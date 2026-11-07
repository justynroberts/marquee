// Package feeds provides live data for the message board: weather (Open-Meteo, no key) and
// PagerDuty incidents. Both register as board token sources.
package feeds

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/justynroberts/marquee/internal/board"
)

// Endpoints are variables so tests can point them at a fake server.
var (
	GeocodeURL  = "https://geocoding-api.open-meteo.com/v1/search"
	ForecastURL = "https://api.open-meteo.com/v1/forecast"
	httpClient  = &http.Client{Timeout: 6 * time.Second}
	userAgent   = "marquee/1.0"
)

const weatherTTL = 15 * time.Minute

type wmo struct{ desc, icon string }

// WMO weather codes -> description and emoji (Open-Meteo "weather_code").
var wmoCodes = map[int]wmo{
	0: {"clear", "☀️"}, 1: {"mostly clear", "🌤️"}, 2: {"partly cloudy", "⛅️"}, 3: {"overcast", "☁️"},
	45: {"fog", "🌫️"}, 48: {"fog", "🌫️"}, 51: {"drizzle", "🌦️"}, 53: {"drizzle", "🌦️"}, 55: {"drizzle", "🌦️"},
	61: {"rain", "🌧️"}, 63: {"rain", "🌧️"}, 65: {"heavy rain", "🌧️"}, 66: {"freezing rain", "🌧️"}, 67: {"freezing rain", "🌧️"},
	71: {"snow", "🌨️"}, 73: {"snow", "🌨️"}, 75: {"heavy snow", "❄️"}, 77: {"snow", "🌨️"},
	80: {"showers", "🌦️"}, 81: {"showers", "🌧️"}, 82: {"heavy showers", "⛈️"}, 85: {"snow showers", "🌨️"}, 86: {"snow showers", "🌨️"},
	95: {"thunderstorm", "⛈️"}, 96: {"thunderstorm", "⛈️"}, 99: {"thunderstorm", "⛈️"},
}

type weatherState struct {
	mu       sync.Mutex
	location string
	at       time.Time
	busy     bool
	text     string
	temp     string
}

var ws weatherState

// Weather returns the cached "{icon} {place} {temp} {description}" and "{temp}" strings for a
// location. It never blocks: a stale or new location starts a background refresh and the previous
// value (or "…") is returned meanwhile.
func Weather(location string) (text, temp string) {
	if location == "" {
		return "set @location=City", "?"
	}
	ws.mu.Lock()
	defer ws.mu.Unlock()
	stale := ws.location != location || time.Since(ws.at) > weatherTTL
	if stale && !ws.busy {
		if ws.location != location {
			ws.text, ws.temp = "…", "…"
		}
		ws.location, ws.busy = location, true
		go func(loc string) {
			t, tp, err := FetchWeather(loc)
			ws.mu.Lock()
			defer ws.mu.Unlock()
			if err == nil {
				ws.text, ws.temp = t, tp
			} else if ws.text == "" || ws.text == "…" {
				ws.text, ws.temp = "weather unavailable", "?"
			}
			ws.at, ws.busy = time.Now(), false
		}(location)
	}
	return ws.text, ws.temp
}

func getJSON(u string, out any) error {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// FetchWeather looks a place up and returns its current weather. Synchronous; for tests and
// the background refresh.
func FetchWeather(location string) (text, temp string, err error) {
	var geo struct {
		Results []struct {
			Name      string  `json:"name"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"results"`
	}
	if err := getJSON(GeocodeURL+"?count=1&name="+url.QueryEscape(location), &geo); err != nil {
		return "", "", err
	}
	if len(geo.Results) == 0 {
		return location + "?", "", nil
	}
	p := geo.Results[0]
	var fc struct {
		Current struct {
			Temperature float64 `json:"temperature_2m"`
			Code        int     `json:"weather_code"`
		} `json:"current"`
	}
	u := fmt.Sprintf("%s?current=temperature_2m,weather_code&timezone=auto&latitude=%g&longitude=%g", ForecastURL, p.Latitude, p.Longitude)
	if err := getJSON(u, &fc); err != nil {
		return "", "", err
	}
	w := wmoCodes[fc.Current.Code]
	temp = fmt.Sprintf("%d°C", int(math.RoundToEven(fc.Current.Temperature))) // Python round(): half to even
	text = temp
	if w.icon != "" {
		text = w.icon + " " + p.Name + " " + temp + " " + w.desc
	} else {
		text = p.Name + " " + temp
	}
	return text, temp, nil
}

func init() {
	board.SetTokenSource("weather", func() string { t, _ := Weather(board.CurrentLocation()); return t })
	board.SetTokenSource("temp", func() string { _, tp := Weather(board.CurrentLocation()); return tp })
}
