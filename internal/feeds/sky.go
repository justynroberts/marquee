package feeds

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

var (
	CarbonURL      = "https://api.carbonintensity.org.uk"
	AuroraWatchURL = "https://aurorawatch-api.lancs.ac.uk/0.2/status"
	KpForecastURL  = "https://services.swpc.noaa.gov/products/noaa-planetary-k-index-forecast.json"
)

// ------------------------------------------------------------------ electricity grid

// Grid is the GB electricity grid now for a region (or the whole country), from the National
// Energy System Operator's Carbon Intensity API.
type Grid struct {
	Region    string // "East Midlands", or "Great Britain"
	Intensity int    // gCO2/kWh
	Index     string // "very low" ... "very high"
	Mix       []Fuel // largest first
	Forecast  []HalfHour
}

type Fuel struct {
	Name string
	Perc float64
}

type HalfHour struct {
	From      time.Time
	Intensity int
	Index     string
}

type carbonPoint struct {
	From      string `json:"from"`
	Intensity struct {
		Forecast int    `json:"forecast"`
		Actual   *int   `json:"actual"`
		Index    string `json:"index"`
	} `json:"intensity"`
	Mix []struct {
		Fuel string  `json:"fuel"`
		Perc float64 `json:"perc"`
	} `json:"generationmix"`
}

// PostcodesURL is postcodes.io, which finds the postcode district nearest a point.
var PostcodesURL = "https://api.postcodes.io"

// NearestOutcode returns the UK postcode district nearest lat, lon (e.g. SW1A).
func NearestOutcode(lat, lon float64) (string, error) {
	var r struct {
		Result []struct {
			Outcode string `json:"outcode"`
		} `json:"result"`
	}
	if err := getJSON(fmt.Sprintf("%s/outcodes?lon=%.2f&lat=%.2f&limit=1", PostcodesURL, lon, lat), &r); err != nil {
		return "", err
	}
	if len(r.Result) == 0 {
		return "", fmt.Errorf("no postcode district near here")
	}
	return r.Result[0].Outcode, nil
}

// FetchGrid returns the grid for a postcode district such as SW1A, a place string of
// coordinates ("@lat,lon|Name", whose district is looked up), or Great Britain when area is "".
func FetchGrid(area string) (*Grid, error) {
	outcode := area
	if p, ok := parseCoords(area); ok {
		oc, err := NearestOutcode(p.Latitude, p.Longitude)
		if err != nil {
			return nil, fmt.Errorf("grid: %v", err)
		}
		outcode = oc
	}
	g := &Grid{Region: "Great Britain"}
	from := time.Now().UTC().Truncate(30 * time.Minute).Format("2006-01-02T15:04Z")
	var now carbonPoint
	var fc []carbonPoint
	if outcode != "" {
		oc := url.PathEscape(strings.ToUpper(outcode))
		var cur struct {
			Data []struct {
				Short string        `json:"shortname"`
				Data  []carbonPoint `json:"data"`
			} `json:"data"`
		}
		if err := getJSON(CarbonURL+"/regional/postcode/"+oc, &cur); err != nil {
			return nil, err
		}
		if len(cur.Data) == 0 || len(cur.Data[0].Data) == 0 {
			return nil, fmt.Errorf("grid: no region for %s", outcode)
		}
		g.Region, now = cur.Data[0].Short, cur.Data[0].Data[0]
		var f struct {
			Data struct {
				Data []carbonPoint `json:"data"`
			} `json:"data"`
		}
		if err := getJSON(CarbonURL+"/regional/intensity/"+from+"/fw48h/postcode/"+oc, &f); err == nil {
			fc = f.Data.Data
		}
	} else {
		var cur struct {
			Data []carbonPoint `json:"data"`
		}
		if err := getJSON(CarbonURL+"/intensity", &cur); err != nil || len(cur.Data) == 0 {
			return nil, fmt.Errorf("grid: %v", err)
		}
		now = cur.Data[0]
		var gen struct {
			Data carbonPoint `json:"data"`
		}
		if err := getJSON(CarbonURL+"/generation", &gen); err == nil {
			now.Mix = gen.Data.Mix
		}
		var f struct {
			Data []carbonPoint `json:"data"`
		}
		if err := getJSON(CarbonURL+"/intensity/"+from+"/fw48h", &f); err == nil {
			fc = f.Data
		}
	}
	g.Intensity, g.Index = now.Intensity.Forecast, now.Intensity.Index
	if now.Intensity.Actual != nil {
		g.Intensity = *now.Intensity.Actual
	}
	for _, m := range now.Mix {
		if m.Perc > 0 {
			g.Mix = append(g.Mix, Fuel{m.Fuel, m.Perc})
		}
	}
	sort.SliceStable(g.Mix, func(i, j int) bool { return g.Mix[i].Perc > g.Mix[j].Perc })
	for _, p := range fc {
		t, err := time.Parse("2006-01-02T15:04Z", p.From)
		if err == nil {
			g.Forecast = append(g.Forecast, HalfHour{t, p.Intensity.Forecast, p.Intensity.Index})
		}
	}
	return g, nil
}

// ------------------------------------------------------------------ aurora

// Aurora is the UK aurora outlook: AuroraWatch UK's status and the last day's disturbance
// (Lancaster University's magnetometers), and NOAA's planetary Kp, observed and forecast.
type Aurora struct {
	Status   string // green, yellow, amber, red
	Activity []AuroraHour
	Kp       []KpPoint
}

type AuroraHour struct {
	At     time.Time
	Value  float64 // nT
	Status string
}

type KpPoint struct {
	At        time.Time
	Kp        float64
	Predicted bool
}

func getXML(u string, out any) error {
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
	}
	return xml.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}

func FetchAurora() (*Aurora, error) {
	a := &Aurora{}
	var act struct {
		Activity []struct {
			Status string  `xml:"status_id,attr"`
			At     string  `xml:"datetime"`
			Value  float64 `xml:"value"`
		} `xml:"activity"`
	}
	if err := getXML(AuroraWatchURL+"/project/awn/sum-activity.xml", &act); err != nil {
		return nil, err
	}
	for _, h := range act.Activity {
		t, err := time.Parse("2006-01-02T15:04:05-0700", h.At)
		if err == nil {
			a.Activity = append(a.Activity, AuroraHour{t, h.Value, h.Status})
		}
	}
	var cur struct {
		Site struct {
			Status string `xml:"status_id,attr"`
		} `xml:"site_status"`
	}
	if err := getXML(AuroraWatchURL+"/current-status.xml", &cur); err == nil {
		a.Status = cur.Site.Status
	}
	if a.Status == "" && len(a.Activity) > 0 {
		a.Status = a.Activity[len(a.Activity)-1].Status
	}
	var kp []struct {
		Tag      string  `json:"time_tag"`
		Kp       float64 `json:"kp"`
		Observed string  `json:"observed"`
	}
	if err := getJSON(KpForecastURL, &kp); err == nil {
		for _, k := range kp {
			t, err := time.Parse("2006-01-02T15:04:05", k.Tag)
			if err == nil {
				a.Kp = append(a.Kp, KpPoint{t, k.Kp, k.Observed != "observed"})
			}
		}
	}
	return a, nil
}
