package feeds

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchRain(t *testing.T) {
	now := time.Now().UTC().Truncate(15 * time.Minute)
	slot := func(d time.Duration) string { return `"` + now.Add(d).Format("2006-01-02T15:04") + `"` }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"utc_offset_seconds":0,"minutely_15":{"time":[` + slot(-15*time.Minute) + `,` + slot(0) + `,` + slot(15*time.Minute) +
			`],"precipitation":[0.5,0.2,0],"precipitation_probability":[50,40,null]},"hourly":{"time":[` + slot(0) + `],"precipitation":[0.7],"precipitation_probability":[60]}}`))
	}))
	defer srv.Close()
	old := ForecastURL
	ForecastURL = srv.URL
	defer func() { ForecastURL = old }()
	rc, err := FetchRain("@51.50,-0.14|Westminster")
	if err != nil {
		t.Fatal(err)
	}
	if rc.Place != "Westminster" || len(rc.Quarters) != 2 || rc.Quarters[0].MM != 0.2 || rc.Quarters[1].Chance != 0 || len(rc.Hours) != 1 || rc.Hours[0].Chance != 60 {
		t.Fatalf("%+v", rc)
	}
}

func TestFetchStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/summary.json" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"status":{"indicator":"minor","description":"Partial System Outage"},
"components":[{"name":"API","status":"operational"},{"name":"Actions","status":"partial_outage"},{"name":"Group","status":"major_outage","group":true}],
"incidents":[{"name":"Actions delayed","status":"investigating","impact":"minor","updated_at":"2026-11-10T12:00:00Z",
"incident_updates":[{"body":"We are\n investigating.","updated_at":"2026-11-10T12:00:00Z"}]},{"name":"Old","status":"resolved"}]}`))
	}))
	defer srv.Close()
	s, err := FetchStatus(StatusPage{Name: "Hub", URL: srv.URL + "/"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Indicator != "minor" || len(s.Down) != 1 || s.Down[0].Name != "Actions" || len(s.Incidents) != 1 || s.Incidents[0].Update != "We are investigating." {
		t.Fatalf("%+v", s)
	}
}

func TestFetchLaunches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"results":[{"name":"Long March 2D | Unknown Payload","net":"2026-11-11T11:40:00Z","status":{"name":"Go for Launch","abbrev":"Go"},
"net_precision":{"name":"Hour"},"launch_service_provider":{"name":"China Aerospace Science and Technology Corporation","abbrev":"CASC"},
"rocket":{"configuration":{"name":"Long March 2D"}},"mission":{"name":"Unknown Payload","orbit":{"name":"Unknown","abbrev":"N/A"}},
"pad":{"name":"Launch Area 94","location":{"name":"Jiuquan Satellite Launch Center"},"country":{"name":"China"}},"webcast_live":false}]}`))
	}))
	defer srv.Close()
	old := LaunchesURL
	LaunchesURL = srv.URL
	defer func() { LaunchesURL = old }()
	ls, err := FetchLaunches()
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) != 1 || ls[0].Rocket != "Long March 2D" || ls[0].Provider != "CASC" || ls[0].Status != "Go" || ls[0].Precision != "Hour" || ls[0].Country != "China" || !ls[0].Net.Equal(time.Date(2026, 11, 11, 11, 40, 0, 0, time.UTC)) {
		t.Fatalf("%+v", ls)
	}
}

func TestFetchRates(t *testing.T) {
	var asked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.String()
		w.Write([]byte(`{"base":"GBP","rates":{"2026-11-10":{"EUR":1.197,"USD":1.3423},"2026-11-07":{"EUR":1.19,"USD":1.34},"2026-11-06":{"EUR":1.18}}}`))
	}))
	defer srv.Close()
	old := FXURL
	FXURL = srv.URL
	defer func() { FXURL = old }()
	rs, err := FetchRates("GBP", []string{"USD", "EUR", "XXX"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(asked, "symbols=USD%2CEUR%2CXXX") || len(rs) != 2 || rs[0].Rate != 1.3423 || rs[0].Prev != 1.34 || len(rs[1].Month) != 3 || rs[1].Date != "2026-11-10" {
		t.Fatalf("%s %+v", asked, rs)
	}
}
