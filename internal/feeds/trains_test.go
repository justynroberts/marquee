package feeds

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// shaped like Darwin's GetArrDepBoardWithDetails JSON
const sampleBoard = `{
 "generatedAt": "2026-11-10T15:02:11.5+00:00", "locationName": "Long Eaton", "crs": "LGE", "platformAvailable": true,
 "trainServices": [
  {"sta": "15:11", "eta": "On time", "std": "15:12", "etd": "15:19", "platform": "2", "operator": "East Midlands Railway",
   "serviceType": "train", "delayReason": "This train has been delayed by a points failure",
   "origin": [{"locationName": "Nottingham", "crs": "NOT"}], "destination": [{"locationName": "London St Pancras International", "crs": "STP"}]},
  {"std": "15:05", "etd": "Cancelled", "operator": "CrossCountry", "serviceType": "train", "isCancelled": true,
   "cancelReason": "This train has been cancelled because of a shortage of train crew",
   "origin": [{"locationName": "Derby", "crs": "DBY"}], "destination": [{"locationName": "Nottingham", "crs": "NOT"}]},
  {"sta": "15:30", "eta": "Delayed", "operator": "East Midlands Railway", "serviceType": "train",
   "origin": [{"locationName": "Leicester", "crs": "LEI"}], "destination": [{"locationName": "Long Eaton", "crs": "LGE"}]},
  {"sta": "23:58", "eta": "00:03", "std": "23:59", "etd": "00:04", "operator": "East Midlands Railway", "serviceType": "train",
   "origin": [{"locationName": "Sheffield", "crs": "SHF"}], "destination": [{"locationName": "Nottingham", "crs": "NOT"}]}
 ],
 "busServices": [
  {"std": "15:40", "etd": "On time", "operator": "East Midlands Railway", "serviceType": "bus",
   "origin": [{"locationName": "Long Eaton", "crs": "LGE"}], "destination": [{"locationName": "Derby", "crs": "DBY"}]}
 ]}`

func TestParseBoard(t *testing.T) {
	var r ldbBoard
	if err := json.Unmarshal([]byte(sampleBoard), &r); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 10, 15, 2, 0, 0, UK)
	b := parseBoard(r, "lge", now)
	if b.Station != "Long Eaton" || len(b.Departures) != 4 || len(b.Arrivals) != 3 {
		t.Fatalf("%s: %d departures, %d arrivals", b.Station, len(b.Departures), len(b.Arrivals))
	}
	if d := b.Departures[0]; !d.Cancelled || d.Place != "Nottingham" || d.Reason == "" {
		t.Errorf("first departure %+v", d)
	}
	if d := b.Departures[1]; d.Late != 7 || d.Platform != "2" || d.Place != "London St Pancras International" {
		t.Errorf("second departure %+v", d)
	}
	if d := b.Departures[2]; !d.Bus || d.Place != "Derby" {
		t.Errorf("bus %+v", d)
	}
	if a := b.Arrivals[1]; !a.Delayed || a.Place != "Leicester" {
		t.Errorf("delayed arrival %+v", a)
	}
	// 23:59 running five minutes late into tomorrow
	if d := b.Departures[3]; d.Late != 5 || d.Expected.Day() != 11 {
		t.Errorf("midnight %+v", d)
	}
}

func TestFetchBoardKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-apikey") != "k1" || r.URL.Path != "/LGE" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(sampleBoard))
	}))
	defer srv.Close()
	old := TrainsURL
	TrainsURL = srv.URL
	defer func() { TrainsURL = old }()
	if b, err := FetchBoard("k1", "lge", 2*time.Hour); err != nil || b.Station != "Long Eaton" {
		t.Fatalf("%v %+v", err, b)
	}
	if _, err := FetchBoard("bad", "LGE", time.Hour); err == nil {
		t.Fatal("a refused key passed")
	}
	if _, err := FetchBoard("", "LGE", time.Hour); err != ErrNoToken {
		t.Fatalf("no key: %v", err)
	}
}
