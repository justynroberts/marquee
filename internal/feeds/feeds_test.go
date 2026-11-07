package feeds

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/justynroberts/marquee/internal/board"
)

func fakeOpenMeteo(t *testing.T, code int, temp float64, place string) func() {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/geo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "marquee/1.0" {
			t.Errorf("user agent %q", r.Header.Get("User-Agent"))
		}
		if place == "" {
			w.Write([]byte(`{}`))
			return
		}
		w.Write([]byte(`{"results":[{"name":"` + place + `","latitude":51.5,"longitude":-0.12}]}`))
	})
	mux.HandleFunc("/fc", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "latitude=51.5") {
			t.Errorf("forecast query %q", r.URL.RawQuery)
		}
		w.Write([]byte(`{"current":{"temperature_2m":` + strconv.FormatFloat(temp, 'f', -1, 64) + `,"weather_code":` + strconv.Itoa(code) + `}}`))
	})
	srv := httptest.NewServer(mux)
	oldG, oldF := GeocodeURL, ForecastURL
	GeocodeURL, ForecastURL = srv.URL+"/geo", srv.URL+"/fc"
	return func() { srv.Close(); GeocodeURL, ForecastURL = oldG, oldF }
}

func TestFetchWeather(t *testing.T) {
	defer fakeOpenMeteo(t, 2, 9.6, "London")()
	text, temp, err := FetchWeather("London")
	if err != nil {
		t.Fatal(err)
	}
	if temp != "10°C" || text != "⛅️ London 10°C partly cloudy" {
		t.Errorf("got %q / %q", text, temp)
	}
}

func TestFetchWeatherUnknownPlace(t *testing.T) {
	defer fakeOpenMeteo(t, 0, 0, "")()
	text, _, err := FetchWeather("Nowhereville")
	if err != nil || text != "Nowhereville?" {
		t.Errorf("got %q, %v", text, err)
	}
}

func TestWeatherNonBlockingAndCached(t *testing.T) {
	defer fakeOpenMeteo(t, 61, 4, "Leeds")()
	ws = weatherState{}
	if text, temp := Weather(""); !strings.Contains(text, "@location") || temp != "?" {
		t.Errorf("no location: %q %q", text, temp)
	}
	text, _ := Weather("Leeds")
	if text != "…" {
		t.Errorf("first call should not block, got %q", text)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if text, temp := Weather("Leeds"); strings.Contains(text, "rain") {
			if temp != "4°C" {
				t.Errorf("temp %q", temp)
			}
			// board tokens go through the same cache
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("weather never arrived")
}

func TestPagerDutyNewIncidents(t *testing.T) {
	os.Setenv("PAGERDUTY_TOKEN", "test")
	defer os.Unsetenv("PAGERDUTY_TOKEN")
	polls := [][]Incident{
		{{ID: "A", Status: "acknowledged", Title: "Disk 90% on db-01"}},
		{{ID: "B", Status: "triggered", Title: "Checkout API 5xx spike", Urgency: "high"}, {ID: "A", Status: "acknowledged", Title: "Disk 90% on db-01"}},
		{{ID: "B", Status: "triggered", Title: "Checkout API 5xx spike", Urgency: "high"}},
	}
	f := NewFeed("")
	f.Fetch = func(tok string) ([]Incident, error) {
		if tok != "test" {
			t.Errorf("token %q", tok)
		}
		p := polls[0]
		polls = polls[1:]
		return p, nil
	}
	var fired []string
	f.OnNew(func(title, urgency string) { fired = append(fired, title+"/"+urgency) })
	f.Poll()
	if len(fired) != 0 {
		t.Errorf("incidents open at the first poll must not fire: %v", fired)
	}
	if tk := f.Tokens(); tk["incidents"] != "1 acknowledged" || tk["incident"] != "Disk 90% on db-01" {
		t.Errorf("tokens after poll 1: %v", tk)
	}
	f.Poll()
	if len(fired) != 1 || fired[0] != "Checkout API 5xx spike/high" {
		t.Errorf("new triggered incident: %v", fired)
	}
	if tk := f.Tokens(); tk["incidents"] != "1 triggered · 1 acknowledged" || tk["incident"] != "Checkout API 5xx spike" {
		t.Errorf("tokens after poll 2: %v", tk)
	}
	f.Poll()
	if len(fired) != 1 {
		t.Errorf("same incident fired twice: %v", fired)
	}
}

func TestPagerDutyTokenSourcesAndErrors(t *testing.T) {
	os.Unsetenv("PAGERDUTY_TOKEN")
	dir := t.TempDir()
	f := NewFeed(filepath.Join(dir, "token"))
	if f.Enabled() || f.Tokens()["incidents"] != "PagerDuty not set up" {
		t.Errorf("no token: %v", f.Tokens())
	}
	os.WriteFile(filepath.Join(dir, "token"), []byte("  filetok \n"), 0o600)
	if f.Token() != "filetok" {
		t.Errorf("token file: %q", f.Token())
	}
	f.Fetch = func(string) ([]Incident, error) { return nil, errors.New("HTTP 401") }
	f.Poll()
	if tk := f.Tokens(); !strings.Contains(tk["incidents"], "401") {
		t.Errorf("error token: %v", tk)
	}
	f.Fetch = func(string) ([]Incident, error) { return nil, nil }
	f.Poll()
	if tk := f.Tokens(); tk["incidents"] != "no open incidents" {
		t.Errorf("empty: %v", tk)
	}
	PollInterval = time.Hour
	f.RegisterTokens()
	defer board.SetTokenSource("incidents", nil)
	defer board.SetTokenSource("incident", nil)
}

func TestFetchIncidentsHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token token=k" || !strings.Contains(r.URL.RawQuery, "statuses%5B%5D=triggered") || !strings.Contains(r.URL.RawQuery, "service_ids%5B%5D=PX1") {
			t.Errorf("request: %v %s", r.Header, r.URL.RawQuery)
		}
		w.Write([]byte(`{"incidents":[{"id":"Q","status":"triggered","title":"T","urgency":"low"}]}`))
	}))
	defer srv.Close()
	os.Setenv("PAGERDUTY_API", srv.URL)
	os.Setenv("PAGERDUTY_SERVICE_IDS", "PX1, ")
	defer os.Unsetenv("PAGERDUTY_API")
	defer os.Unsetenv("PAGERDUTY_SERVICE_IDS")
	inc, err := FetchIncidents("k")
	if err != nil || len(inc) != 1 || inc[0].Title != "T" {
		t.Fatalf("%v %v", inc, err)
	}
}
