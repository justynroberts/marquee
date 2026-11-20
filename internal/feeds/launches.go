package feeds

import (
	"strings"
	"time"
)

// LaunchesURL is The Space Devs' Launch Library 2 (free, about 15 requests an hour).
var LaunchesURL = "https://ll.thespacedevs.com/2.3.0/launches/upcoming/?limit=8&mode=normal"

// Launch is one upcoming rocket launch.
type Launch struct {
	Name      string // "Falcon 9 Block 5 | Starlink Group 10-12"
	Rocket    string
	Mission   string
	Orbit     string
	Provider  string // abbreviation where there is one
	Pad       string
	Location  string
	Country   string
	Net       time.Time // no earlier than: the planned T-0
	Precision string    // "Second", "Minute", "Hour", "Day", "Month"...
	Status    string    // Go, TBD, TBC, Hold, In Flight, Success...
	Webcast   bool
}

// FetchLaunches returns the next launches, soonest first.
func FetchLaunches() ([]Launch, error) {
	type named struct {
		Name   string `json:"name"`
		Abbrev string `json:"abbrev"`
	}
	var r struct {
		Results []struct {
			Name      string    `json:"name"`
			Net       time.Time `json:"net"`
			Status    named     `json:"status"`
			Precision *named    `json:"net_precision"`
			Provider  named     `json:"launch_service_provider"`
			Rocket    struct {
				Configuration struct {
					Name string `json:"name"`
				} `json:"configuration"`
			} `json:"rocket"`
			Mission *struct {
				Name  string `json:"name"`
				Orbit *named `json:"orbit"`
			} `json:"mission"`
			Pad struct {
				Name     string `json:"name"`
				Location struct {
					Name string `json:"name"`
				} `json:"location"`
				Country *struct {
					Name string `json:"name"`
				} `json:"country"`
			} `json:"pad"`
			Webcast bool `json:"webcast_live"`
		} `json:"results"`
	}
	if err := getJSON(LaunchesURL, &r); err != nil {
		return nil, err
	}
	out := make([]Launch, 0, len(r.Results))
	for _, x := range r.Results {
		l := Launch{Name: x.Name, Rocket: x.Rocket.Configuration.Name, Pad: x.Pad.Name, Location: x.Pad.Location.Name,
			Net: x.Net, Status: x.Status.Abbrev, Webcast: x.Webcast}
		if l.Status == "" {
			l.Status = x.Status.Name
		}
		l.Provider = x.Provider.Abbrev
		if l.Provider == "" {
			l.Provider = x.Provider.Name
		}
		if x.Precision != nil {
			l.Precision = x.Precision.Name
		}
		if x.Mission != nil {
			l.Mission = x.Mission.Name
			if x.Mission.Orbit != nil {
				l.Orbit = x.Mission.Orbit.Abbrev
			}
		}
		if l.Mission == "" {
			if _, m, ok := strings.Cut(x.Name, " | "); ok {
				l.Mission = m
			}
		}
		if x.Pad.Country != nil {
			l.Country = x.Pad.Country.Name
		}
		out = append(out, l)
	}
	return out, nil
}
