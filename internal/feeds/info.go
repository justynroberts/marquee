package feeds

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

var (
	CoinsURL     = "https://api.coingecko.com/api/v3/coins/markets"
	AirURL       = "https://air-quality-api.open-meteo.com/v1/air-quality"
	HolidaysURL  = "https://www.gov.uk/bank-holidays.json"
	HolidayAreas = []string{"england-and-wales", "scotland", "northern-ireland"}
)

// ------------------------------------------------------------------ markets

// Coin is one cryptocurrency's price from CoinGecko.
type Coin struct {
	ID, Symbol, Name string
	Price            float64
	Change24h        float64   // percent
	Week             []float64 // hourly prices over the last seven days
	High24h, Low24h  float64
	Cap              float64
}

// FetchCoins returns the coins with these CoinGecko ids, priced in currency ("usd", "gbp"...),
// in the order asked.
func FetchCoins(ids []string, currency string) ([]Coin, error) {
	var resp []struct {
		ID     string   `json:"id"`
		Symbol string   `json:"symbol"`
		Name   string   `json:"name"`
		Price  float64  `json:"current_price"`
		Change *float64 `json:"price_change_percentage_24h"`
		High   float64  `json:"high_24h"`
		Low    float64  `json:"low_24h"`
		Cap    float64  `json:"market_cap"`
		Spark  struct {
			Price []float64 `json:"price"`
		} `json:"sparkline_in_7d"`
	}
	u := fmt.Sprintf("%s?vs_currency=%s&ids=%s&sparkline=true", CoinsURL, url.QueryEscape(currency), url.QueryEscape(strings.Join(ids, ",")))
	if err := getJSON(u, &resp); err != nil {
		return nil, err
	}
	byID := map[string]Coin{}
	for _, c := range resp {
		cn := Coin{ID: c.ID, Symbol: strings.ToUpper(c.Symbol), Name: c.Name, Price: c.Price, High24h: c.High, Low24h: c.Low, Cap: c.Cap, Week: c.Spark.Price}
		if c.Change != nil {
			cn.Change24h = *c.Change
		}
		byID[c.ID] = cn
	}
	var out []Coin
	for _, id := range ids {
		if c, ok := byID[id]; ok {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("markets: none of %s found on CoinGecko", strings.Join(ids, ", "))
	}
	return out, nil
}

// ------------------------------------------------------------------ air quality

// Air is the air quality now and over the next two days at a place, from Open-Meteo (CAMS).
type Air struct {
	AQI                 float64 // European AQI
	PM25, PM10, NO2, O3 float64 // µg/m³
	Pollen              map[string]float64
	Hours               []float64 // European AQI, from the current hour, 48 entries
	HourStart           int       // the first hour's hour of day
}

// PollenKinds are the pollen types Open-Meteo reports (Europe only), in display order.
var PollenKinds = []string{"grass", "birch", "alder", "olive", "mugwort", "ragweed"}

func FetchAir(lat, lon float64) (*Air, error) {
	var pollenFields []string
	for _, k := range PollenKinds {
		pollenFields = append(pollenFields, k+"_pollen")
	}
	u := fmt.Sprintf("%s?latitude=%g&longitude=%g&timezone=auto&forecast_days=3&current=european_aqi,pm2_5,pm10,nitrogen_dioxide,ozone,%s&hourly=european_aqi",
		AirURL, lat, lon, strings.Join(pollenFields, ","))
	var resp struct {
		Current map[string]any `json:"current"`
		Hourly  struct {
			Time []string   `json:"time"`
			AQI  []*float64 `json:"european_aqi"`
		} `json:"hourly"`
	}
	if err := getJSON(u, &resp); err != nil {
		return nil, err
	}
	num := func(k string) (float64, bool) {
		v, ok := resp.Current[k].(float64)
		return v, ok
	}
	a := &Air{Pollen: map[string]float64{}}
	var ok bool
	if a.AQI, ok = num("european_aqi"); !ok {
		return nil, fmt.Errorf("air quality: no reading here")
	}
	a.PM25, _ = num("pm2_5")
	a.PM10, _ = num("pm10")
	a.NO2, _ = num("nitrogen_dioxide")
	a.O3, _ = num("ozone")
	for _, k := range PollenKinds {
		if v, ok := num(k + "_pollen"); ok {
			a.Pollen[k] = v
		}
	}
	now, _ := resp.Current["time"].(string)
	if len(now) >= 13 {
		now = now[:13]
	}
	for i, ts := range resp.Hourly.Time {
		if len(a.Hours) == 48 {
			break
		}
		if len(ts) < 13 || ts[:13] < now || i >= len(resp.Hourly.AQI) {
			continue
		}
		v := 0.0
		if p := resp.Hourly.AQI[i]; p != nil {
			v = *p
		}
		if len(a.Hours) == 0 {
			fmt.Sscanf(ts[11:13], "%d", &a.HourStart)
		}
		a.Hours = append(a.Hours, v)
	}
	return a, nil
}

// AQIBand names a European AQI value and says whether it is poor or worse.
func AQIBand(v float64) (name string, bad bool) {
	switch {
	case v < 20:
		return "Good", false
	case v < 40:
		return "Fair", false
	case v < 60:
		return "Moderate", false
	case v < 80:
		return "Poor", true
	case v < 100:
		return "Very poor", true
	default:
		return "Extremely poor", true
	}
}

// ------------------------------------------------------------------ bank holidays

// Holiday is a UK bank holiday.
type Holiday struct {
	Title string
	Date  time.Time
}

// FetchHolidays returns the bank holidays for one of HolidayAreas, in date order.
func FetchHolidays(area string) ([]Holiday, error) {
	var resp map[string]struct {
		Events []struct {
			Title string `json:"title"`
			Date  string `json:"date"`
		} `json:"events"`
	}
	if err := getJSON(HolidaysURL, &resp); err != nil {
		return nil, err
	}
	d, ok := resp[area]
	if !ok {
		return nil, fmt.Errorf("bank holidays: no area %q (use %s)", area, strings.Join(HolidayAreas, ", "))
	}
	var out []Holiday
	for _, e := range d.Events {
		t, err := time.ParseInLocation("2006-01-02", e.Date, time.Local)
		if err == nil {
			out = append(out, Holiday{strings.ReplaceAll(e.Title, "’", "'"), t})
		}
	}
	return out, nil
}
