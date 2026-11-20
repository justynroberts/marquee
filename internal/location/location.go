// Package location finds where this Mac is, through Core Location: one coarse fix (about a
// kilometre), named by Apple's reverse geocoder, asked for when something first needs it and
// again every few hours. The last fix is kept in a file so a restart has it at once.
package location

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"time"
)

// Fix is a position and the name of the town it is in.
type Fix struct {
	Lat     float64   `json:"lat"`
	Lon     float64   `json:"lon"`
	Name    string    `json:"name"`
	Outcode string    `json:"outcode,omitempty"` // postcode district, e.g. SW1A (never the full postcode)
	At      time.Time `json:"at"`
}

// Place is the fix as a place string feeds.Geocode reads without a lookup: "@lat,lon|Name".
// Coordinates are rounded to two decimals (about a kilometre) before they go anywhere.
func (f Fix) Place() string {
	return fmt.Sprintf("@%.2f,%.2f|%s", math.Round(f.Lat*100)/100, math.Round(f.Lon*100)/100, f.Name)
}

// ErrDenied means location access is turned off for Marquee.
var ErrDenied = errors.New("location access is off for Marquee (System Settings > Privacy & Security > Location Services)")

const refresh = 3 * time.Hour

var (
	mu      sync.Mutex
	cache   string
	last    Fix
	have    bool
	lastErr error
	asked   time.Time
)

// SetCacheFile names the file the last fix is kept in, and loads it.
func SetCacheFile(path string) {
	mu.Lock()
	defer mu.Unlock()
	cache = path
	if b, err := os.ReadFile(path); err == nil {
		var f Fix
		if json.Unmarshal(b, &f) == nil && (f.Lat != 0 || f.Lon != 0) {
			last, have = f, true
		}
	}
}

// Current returns the latest fix (live, or the saved one), asking Core Location for a new one
// when there is none or it is a few hours old. ok is false when there is no fix at all; err
// then says why, if known.
func Current() (f Fix, ok bool, err error) {
	mu.Lock()
	defer mu.Unlock()
	if fx, e, done := poll(); done {
		if e == nil && (fx.Lat != 0 || fx.Lon != 0) {
			fx.At = last.At
			if !have || fx.Lat != last.Lat || fx.Lon != last.Lon || fx.Name != last.Name || fx.Outcode != last.Outcode {
				fx.At = time.Now()
				last, have = fx, true
				save()
			}
		}
		lastErr = e
	}
	if asked.IsZero() || (time.Since(asked) > refresh) {
		asked = time.Now()
		request()
	}
	return last, have, lastErr
}

func save() {
	if cache == "" {
		return
	}
	b, _ := json.MarshalIndent(last, "", "  ")
	_ = os.WriteFile(cache, b, 0o600)
}
