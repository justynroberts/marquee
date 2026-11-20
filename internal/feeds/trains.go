package feeds

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// TrainsURL is National Rail's Live Arrival and Departure Boards (Darwin) on the Rail Data
// Marketplace; the free product's consumer key goes in an x-apikey header. Tests point it
// elsewhere.
var TrainsURL = "https://api1.raildata.org.uk/1010-live-arrival-and-departure-boards-arr-and-dep1_1/LDBWS/api/20220120/GetArrDepBoardWithDetails"

// TrainCall is one train at the station: a departure or an arrival.
type TrainCall struct {
	Time      time.Time // advertised (public timetable) time
	Expected  time.Time // forecast, or actual once it has happened; zero when unknown
	Late      int       // minutes late (negative early); 0 when on time or unknown
	Cancelled bool
	Place     string // destination for a departure, origin for an arrival
	Platform  string
	Operator  string
	Delayed   bool   // late with no estimate yet
	Reason    string // delay or cancellation reason
	Bus       bool   // a replacement bus
}

// Board is a station's departures and arrivals, each in time order.
type Board struct {
	Station    string
	Departures []TrainCall
	Arrivals   []TrainCall
}

// ErrNoToken means no Rail Data Marketplace key has been saved.
var ErrNoToken = errors.New("no rail data marketplace key")

type ldbLocation struct {
	Name string `json:"locationName"`
}

type ldbService struct {
	STA          string        `json:"sta"`
	ETA          string        `json:"eta"`
	STD          string        `json:"std"`
	ETD          string        `json:"etd"`
	Platform     string        `json:"platform"`
	Operator     string        `json:"operator"`
	ServiceType  string        `json:"serviceType"`
	Cancelled    bool          `json:"isCancelled"`
	CancelReason string        `json:"cancelReason"`
	DelayReason  string        `json:"delayReason"`
	Origin       []ldbLocation `json:"origin"`
	Destination  []ldbLocation `json:"destination"`
}

type ldbBoard struct {
	LocationName string       `json:"locationName"`
	Trains       []ldbService `json:"trainServices"`
	Buses        []ldbService `json:"busServices"`
}

// FetchBoard returns the trains at station (a CRS code such as LGE) over the next window (up
// to two hours).
func FetchBoard(key, station string, window time.Duration) (*Board, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, ErrNoToken
	}
	q := url.Values{"numRows": {"30"}, "timeWindow": {fmt.Sprint(int(min(window, 2*time.Hour).Minutes()))}}
	req, _ := http.NewRequest(http.MethodGet, TrainsURL+"/"+url.PathEscape(strings.ToUpper(station))+"?"+q.Encode(), nil)
	req.Header.Set("x-apikey", key)
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, errors.New("train times: the rail data marketplace key was refused (new keys can take 30 minutes to work)")
	default:
		return nil, fmt.Errorf("train times: HTTP %d", resp.StatusCode)
	}
	var b ldbBoard
	if err := json.NewDecoder(resp.Body).Decode(&b); err != nil {
		return nil, fmt.Errorf("train times: %v", err)
	}
	return parseBoard(b, station, time.Now()), nil
}

// UK is where Darwin's "HH:MM" times are.
var UK = func() *time.Location {
	if l, err := time.LoadLocation("Europe/London"); err == nil {
		return l
	}
	return time.UTC
}()

// clockTime places "HH:MM" on the day nearest now, so a board straddling midnight reads right.
func clockTime(hm string, now time.Time) (time.Time, bool) {
	t, err := time.ParseInLocation("15:04", hm, UK)
	if err != nil {
		return time.Time{}, false
	}
	n := now.In(UK)
	at := time.Date(n.Year(), n.Month(), n.Day(), t.Hour(), t.Minute(), 0, 0, UK)
	switch {
	case at.Sub(n) > 12*time.Hour:
		at = at.AddDate(0, 0, -1)
	case n.Sub(at) > 12*time.Hour:
		at = at.AddDate(0, 0, 1)
	}
	return at, true
}

// shortReason drops Darwin's "This train has been delayed by" preamble.
func shortReason(s string) string {
	for _, p := range []string{"This train has been delayed by ", "This train has been cancelled because of ", "This service has been delayed by ", "This service has been cancelled because of "} {
		if r, ok := strings.CutPrefix(s, p); ok {
			return r
		}
	}
	return s
}

func names(ls []ldbLocation) string {
	var n []string
	for _, l := range ls {
		if l.Name != "" {
			n = append(n, l.Name)
		}
	}
	return strings.Join(n, " & ")
}

func parseBoard(b ldbBoard, station string, now time.Time) *Board {
	out := &Board{Station: b.LocationName}
	if out.Station == "" {
		out.Station = strings.ToUpper(station)
	}
	all := append(append([]ldbService{}, b.Trains...), b.Buses...)
	for _, s := range all {
		base := TrainCall{Platform: s.Platform, Operator: s.Operator, Cancelled: s.Cancelled, Bus: s.ServiceType == "bus"}
		base.Reason = shortReason(s.DelayReason)
		if s.Cancelled && s.CancelReason != "" {
			base.Reason = shortReason(s.CancelReason)
		}
		call := func(sched, est, place string) (TrainCall, bool) {
			at, ok := clockTime(sched, now)
			if !ok {
				return TrainCall{}, false
			}
			c := base
			c.Time, c.Place = at, place
			switch est {
			case "", "On time":
			case "Cancelled":
				c.Cancelled = true
			case "Delayed":
				c.Delayed = true
			default:
				if e, ok := clockTime(est, now); ok {
					c.Expected = e
					c.Late = int(e.Sub(at).Minutes())
				}
			}
			return c, true
		}
		if c, ok := call(s.STD, s.ETD, names(s.Destination)); ok {
			out.Departures = append(out.Departures, c)
		}
		if c, ok := call(s.STA, s.ETA, names(s.Origin)); ok {
			out.Arrivals = append(out.Arrivals, c)
		}
	}
	sort.SliceStable(out.Departures, func(i, j int) bool { return out.Departures[i].Time.Before(out.Departures[j].Time) })
	sort.SliceStable(out.Arrivals, func(i, j int) bool { return out.Arrivals[i].Time.Before(out.Arrivals[j].Time) })
	return out
}
