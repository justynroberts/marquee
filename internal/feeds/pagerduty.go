package feeds

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/justynroberts/marquee/internal/board"
)

// PagerDuty open incidents. Needs a REST API key (read-only is enough) from PAGERDUTY_TOKEN or
// a token file. PAGERDUTY_API selects the region (https://api.eu.pagerduty.com for EU accounts);
// PAGERDUTY_SERVICE_IDS (comma-separated) limits the services watched.

// PollInterval is how often Start polls.
var PollInterval = 60 * time.Second

// Incident is the part of a PagerDuty incident the panel uses.
type Incident struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Title   string `json:"title"`
	Urgency string `json:"urgency"`
}

// Feed polls PagerDuty for triggered and acknowledged incidents.
type Feed struct {
	// TokenPath is a file holding the API key, used when PAGERDUTY_TOKEN is not set.
	TokenPath string
	// Fetch is the HTTP call; replace it in tests.
	Fetch func(token string) ([]Incident, error)

	mu        sync.Mutex
	incidents []Incident
	err       string
	updated   time.Time
	seen      map[string]bool // nil until the first successful poll
	onNew     func(title, urgency string)
	started   bool
}

// NewFeed returns a feed reading its token from env or tokenPath.
func NewFeed(tokenPath string) *Feed {
	return &Feed{TokenPath: tokenPath, Fetch: FetchIncidents}
}

// Token returns the API key, or "".
func (f *Feed) Token() string {
	if t := strings.TrimSpace(os.Getenv("PAGERDUTY_TOKEN")); t != "" {
		return t
	}
	if f.TokenPath != "" {
		if b, err := os.ReadFile(f.TokenPath); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return ""
}

// Enabled reports whether a token is configured.
func (f *Feed) Enabled() bool { return f.Token() != "" }

// OnNew sets the callback for newly triggered incidents (never for ones present at the first poll).
func (f *Feed) OnNew(fn func(title, urgency string)) {
	f.mu.Lock()
	f.onNew = fn
	f.mu.Unlock()
}

// Poll fetches once and fires OnNew for new triggered incidents.
func (f *Feed) Poll() {
	tok := f.Token()
	if tok == "" {
		f.mu.Lock()
		f.err = "no PagerDuty token"
		f.mu.Unlock()
		return
	}
	incidents, err := f.Fetch(tok)
	f.mu.Lock()
	if err != nil {
		f.err = "PagerDuty: " + err.Error()
		f.mu.Unlock()
		return
	}
	f.err = ""
	f.incidents = incidents
	f.updated = time.Now()
	var fresh []Incident
	if f.seen != nil {
		for _, inc := range incidents {
			if !f.seen[inc.ID] && inc.Status == "triggered" {
				fresh = append(fresh, inc)
			}
		}
	} else {
		f.seen = map[string]bool{}
	}
	for _, inc := range incidents {
		f.seen[inc.ID] = true
	}
	cb := f.onNew
	f.mu.Unlock()
	if cb != nil {
		for _, inc := range fresh {
			cb(inc.Title, inc.Urgency)
		}
	}
}

// Start polls in the background every PollInterval. Safe to call more than once.
func (f *Feed) Start() {
	f.mu.Lock()
	if f.started {
		f.mu.Unlock()
		return
	}
	f.started = true
	f.mu.Unlock()
	go func() {
		for {
			f.Poll()
			time.Sleep(PollInterval)
		}
	}()
}

// Tokens returns {incidents} ("2 triggered · 1 acknowledged") and {incident} (newest title).
func (f *Feed) Tokens() map[string]string {
	if !f.Enabled() {
		return map[string]string{"incidents": "PagerDuty not set up", "incident": ""}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != "" && f.updated.IsZero() {
		e := f.err
		if len(e) > 60 {
			e = e[:60]
		}
		return map[string]string{"incidents": e, "incident": ""}
	}
	var trig, ack []Incident
	for _, i := range f.incidents {
		switch i.Status {
		case "triggered":
			trig = append(trig, i)
		case "acknowledged":
			ack = append(ack, i)
		}
	}
	var parts []string
	if len(trig) > 0 {
		parts = append(parts, fmt.Sprintf("%d triggered", len(trig)))
	}
	if len(ack) > 0 {
		parts = append(parts, fmt.Sprintf("%d acknowledged", len(ack)))
	}
	summary := "no open incidents"
	if len(parts) > 0 {
		summary = strings.Join(parts, " · ")
	}
	newest := ""
	if len(trig) > 0 {
		newest = trig[0].Title
	} else if len(ack) > 0 {
		newest = ack[0].Title
	}
	if len([]rune(newest)) > 120 {
		newest = string([]rune(newest)[:120])
	}
	return map[string]string{"incidents": summary, "incident": newest}
}

// State is a snapshot for the API.
func (f *Feed) State() (enabled bool, errText string, tokens map[string]string) {
	tokens = f.Tokens()
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Enabled(), f.err, tokens
}

// RegisterTokens makes {incidents} and {incident} available on the board; the first use starts
// polling.
func (f *Feed) RegisterTokens() {
	get := func(k string) func() string {
		return func() string {
			if f.Enabled() {
				f.Start()
			}
			return f.Tokens()[k]
		}
	}
	board.SetTokenSource("incidents", get("incidents"))
	board.SetTokenSource("incident", get("incident"))
}

// FetchIncidents calls the PagerDuty REST API for open incidents.
func FetchIncidents(token string) ([]Incident, error) {
	base := strings.TrimRight(os.Getenv("PAGERDUTY_API"), "/")
	if base == "" {
		base = "https://api.pagerduty.com"
	}
	q := url.Values{}
	q.Add("statuses[]", "triggered")
	q.Add("statuses[]", "acknowledged")
	q.Set("limit", "50")
	q.Set("sort_by", "created_at:desc")
	for _, sid := range strings.Split(os.Getenv("PAGERDUTY_SERVICE_IDS"), ",") {
		if s := strings.TrimSpace(sid); s != "" {
			q.Add("service_ids[]", s)
		}
	}
	req, err := http.NewRequest(http.MethodGet, base+"/incidents?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Token token="+token)
	req.Header.Set("Accept", "application/vnd.pagerduty+json;version=2")
	req.Header.Set("User-Agent", userAgent)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var body struct {
		Incidents []Incident `json:"incidents"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body.Incidents, nil
}
