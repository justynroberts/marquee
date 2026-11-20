package feeds

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// FXURL is Frankfurter: the European Central Bank's reference rates, free with no key.
var FXURL = "https://api.frankfurter.dev/v1"

// Rate is one currency against the base: the latest rate and the last month of daily rates,
// oldest first (working days only, as the ECB publishes them).
type Rate struct {
	Code  string
	Rate  float64
	Prev  float64 // the rate the working day before
	Month []float64
	Date  string // of the latest rate
}

// FetchRates returns base against each quote, in the order asked.
func FetchRates(base string, quotes []string) ([]Rate, error) {
	if len(quotes) == 0 {
		return nil, fmt.Errorf("fx: no currencies")
	}
	from := time.Now().AddDate(0, 0, -32).Format("2006-01-02")
	var r struct {
		Rates map[string]map[string]float64 `json:"rates"`
	}
	u := fmt.Sprintf("%s/%s..?base=%s&symbols=%s", FXURL, from, url.QueryEscape(base), url.QueryEscape(strings.Join(quotes, ",")))
	if err := getJSON(u, &r); err != nil {
		return nil, err
	}
	days := make([]string, 0, len(r.Rates))
	for d := range r.Rates {
		days = append(days, d)
	}
	if len(days) == 0 {
		return nil, fmt.Errorf("fx: no rates for %s", base)
	}
	sort.Strings(days)
	out := make([]Rate, 0, len(quotes))
	for _, q := range quotes {
		rt := Rate{Code: q}
		for _, d := range days {
			if v, ok := r.Rates[d][q]; ok {
				rt.Month = append(rt.Month, v)
				rt.Date = d
			}
		}
		if n := len(rt.Month); n > 0 {
			rt.Rate, rt.Prev = rt.Month[n-1], rt.Month[n-1]
			if n > 1 {
				rt.Prev = rt.Month[n-2]
			}
			out = append(out, rt)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("fx: no rates for %s", strings.Join(quotes, ", "))
	}
	return out, nil
}
