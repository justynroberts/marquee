package feeds

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Forecast is a place's current weather, the next 24 hours and the coming days, in the place's
// own time zone.
type Forecast struct {
	Place           string
	Temp, Feels     float64
	Humidity, Wind  float64 // %, km/h
	Code            int
	Hours           []HourCast // from the current hour, 24 entries
	Days            []DayCast  // today first
	Sunrise, Sunset string     // "07:12"
	Fetched         time.Time
}

type HourCast struct {
	Hour int // 0-23, local to the place
	Temp float64
	Rain int // precipitation probability, %
	Code int
}

type DayCast struct {
	Day    string // "MON"
	Hi, Lo float64
	Rain   int
	Code   int
}

// Describe returns a short upper-case description of a WMO weather code.
func Describe(code int) string {
	if w, ok := wmoCodes[code]; ok {
		return strings.ToUpper(w.desc)
	}
	return ""
}

// Place is a geocoded location.
type Place struct {
	Name      string
	Latitude  float64
	Longitude float64
}

// Geocode looks a place name up with Open-Meteo.
func Geocode(location string) (Place, error) {
	if p, ok := parseCoords(location); ok {
		return p, nil
	}
	if pc, ok := UKPostcode(location); ok {
		return geocodePostcode(pc)
	}
	var geo struct {
		Results []struct {
			Name      string  `json:"name"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"results"`
	}
	if err := getJSON(GeocodeURL+"?count=1&name="+url.QueryEscape(location), &geo); err != nil {
		return Place{}, err
	}
	if len(geo.Results) == 0 {
		return Place{}, fmt.Errorf("no place called %q", location)
	}
	r := geo.Results[0]
	return Place{r.Name, r.Latitude, r.Longitude}, nil
}

var postcodeRe = regexp.MustCompile(`^([A-Z]{1,2}[0-9][A-Z0-9]?) ?([0-9][A-Z]{2})$`)

// UKPostcode recognises a full UK postcode ("SW1A 1AA", any case or spacing) and returns it
// normalised.
func UKPostcode(s string) (string, bool) {
	m := postcodeRe.FindStringSubmatch(strings.ToUpper(strings.Join(strings.Fields(s), " ")))
	if m == nil {
		m = postcodeRe.FindStringSubmatch(strings.ToUpper(strings.ReplaceAll(s, " ", "")))
	}
	if m == nil {
		return "", false
	}
	return m[1] + " " + m[2], true
}

// Outcode is a postcode's district: "SW1A" for "SW1A 1AA".
func Outcode(postcode string) string {
	o, _, _ := strings.Cut(postcode, " ")
	return o
}

// geocodePostcode looks a UK postcode up with postcodes.io and names it by its parish, ward or
// district.
func geocodePostcode(pc string) (Place, error) {
	var r struct {
		Result struct {
			Lat      float64 `json:"latitude"`
			Lon      float64 `json:"longitude"`
			Parish   string  `json:"parish"`
			Ward     string  `json:"admin_ward"`
			District string  `json:"admin_district"`
		} `json:"result"`
	}
	if err := getJSON(PostcodesURL+"/postcodes/"+url.PathEscape(strings.ReplaceAll(pc, " ", "")), &r); err != nil {
		return Place{}, fmt.Errorf("postcode %s: %v", pc, err)
	}
	name := r.Result.Parish
	if name == "" || strings.Contains(name, "unparished") {
		name = r.Result.Ward
	}
	if name == "" {
		name = r.Result.District
	}
	return Place{Name: name, Latitude: r.Result.Lat, Longitude: r.Result.Lon}, nil
}

// parseCoords reads "@lat,lon|Name" (this Mac's own location, already known), which needs no
// lookup.
func parseCoords(s string) (Place, bool) {
	if !strings.HasPrefix(s, "@") {
		return Place{}, false
	}
	coords, name, _ := strings.Cut(s[1:], "|")
	var p Place
	if _, err := fmt.Sscanf(coords, "%g,%g", &p.Latitude, &p.Longitude); err != nil {
		return Place{}, false
	}
	p.Name = name
	if p.Name == "" {
		p.Name = "Here"
	}
	return p, true
}

// FetchForecast looks a place up and fetches its forecast.
func FetchForecast(location string) (*Forecast, error) {
	p, err := Geocode(location)
	if err != nil {
		return nil, err
	}
	var fc struct {
		Current struct {
			Time     string  `json:"time"`
			Temp     float64 `json:"temperature_2m"`
			Feels    float64 `json:"apparent_temperature"`
			Humidity float64 `json:"relative_humidity_2m"`
			Wind     float64 `json:"wind_speed_10m"`
			Code     int     `json:"weather_code"`
		} `json:"current"`
		Hourly struct {
			Time []string  `json:"time"`
			Temp []float64 `json:"temperature_2m"`
			Rain []int     `json:"precipitation_probability"`
			Code []int     `json:"weather_code"`
		} `json:"hourly"`
		Daily struct {
			Time    []string  `json:"time"`
			Code    []int     `json:"weather_code"`
			Hi      []float64 `json:"temperature_2m_max"`
			Lo      []float64 `json:"temperature_2m_min"`
			Rain    []int     `json:"precipitation_probability_max"`
			Sunrise []string  `json:"sunrise"`
			Sunset  []string  `json:"sunset"`
		} `json:"daily"`
	}
	u := fmt.Sprintf("%s?latitude=%g&longitude=%g&timezone=auto&forecast_days=6"+
		"&current=temperature_2m,apparent_temperature,relative_humidity_2m,wind_speed_10m,weather_code"+
		"&hourly=temperature_2m,precipitation_probability,weather_code"+
		"&daily=weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max,sunrise,sunset",
		ForecastURL, p.Latitude, p.Longitude)
	if err := getJSON(u, &fc); err != nil {
		return nil, err
	}
	f := &Forecast{
		Place: p.Name, Temp: fc.Current.Temp, Feels: fc.Current.Feels,
		Humidity: fc.Current.Humidity, Wind: fc.Current.Wind, Code: fc.Current.Code, Fetched: time.Now(),
	}
	// hourly times are "2026-11-10T14:00"; start at the current hour
	now := fc.Current.Time
	if len(now) >= 13 {
		now = now[:13]
	}
	h := fc.Hourly
	for i, ts := range h.Time {
		if len(f.Hours) == 24 {
			break
		}
		if len(ts) < 13 || ts[:13] < now || i >= len(h.Temp) || i >= len(h.Rain) || i >= len(h.Code) {
			continue
		}
		var hr int
		fmt.Sscanf(ts[11:13], "%d", &hr)
		f.Hours = append(f.Hours, HourCast{Hour: hr, Temp: h.Temp[i], Rain: h.Rain[i], Code: h.Code[i]})
	}
	d := fc.Daily
	for i, ds := range d.Time {
		if i >= len(d.Hi) || i >= len(d.Lo) || i >= len(d.Code) || i >= len(d.Rain) {
			break
		}
		day, err := time.Parse("2006-01-02", ds)
		if err != nil {
			continue
		}
		f.Days = append(f.Days, DayCast{Day: strings.ToUpper(day.Format("Mon")), Hi: d.Hi[i], Lo: d.Lo[i], Rain: d.Rain[i], Code: d.Code[i]})
	}
	if len(d.Sunrise) > 0 && len(d.Sunrise[0]) >= 16 {
		f.Sunrise = d.Sunrise[0][11:16]
	}
	if len(d.Sunset) > 0 && len(d.Sunset[0]) >= 16 {
		f.Sunset = d.Sunset[0][11:16]
	}
	return f, nil
}
