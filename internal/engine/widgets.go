package engine

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/justynroberts/marquee/internal/dashboard"
	"github.com/justynroberts/marquee/internal/led"
)

// Widgets are small labelled readings that anything can push: a script, a cron job, a CI step,
// or a cloud tool through the relay. The widgets screen lays them out as a board. They are
// kept in widgets.json in the data folder.

// Widget is one reading.
type Widget struct {
	ID      string    `json:"id"`
	Label   string    `json:"label,omitempty"`
	Value   string    `json:"value,omitempty"`
	Unit    string    `json:"unit,omitempty"`
	Status  string    `json:"status,omitempty"` // ok, warn, bad, info
	Text    string    `json:"text,omitempty"`
	TTL     float64   `json:"ttl,omitempty"` // seconds until it is dropped; 0 keeps it
	Flash   bool      `json:"flash,omitempty"`
	Updated time.Time `json:"updated"`
}

const maxWidgets = 24

var widgetID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,39}$`)

// ErrBadWidget is returned for a widget that cannot be stored.
var ErrBadWidget = errors.New("bad widget")

func (e *Engine) widgetsFile() string { return filepath.Join(e.paths.Dir, "widgets.json") }

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// SetWidget adds or replaces a widget (by id) and saves the board. A widget with flash set
// also flashes its label and value or text on the panel.
func (e *Engine) SetWidget(w Widget) (Widget, error) {
	w.ID = strings.ToLower(strings.TrimSpace(w.ID))
	if w.ID == "" {
		w.ID = slug(w.Label)
	}
	if !widgetID.MatchString(w.ID) {
		return w, fmt.Errorf("%w: id must be 1-40 lowercase letters, digits, dot, dash or underscore", ErrBadWidget)
	}
	w.Label, w.Value, w.Unit, w.Text = clip(w.Label, 40), clip(w.Value, 24), clip(w.Unit, 12), clip(w.Text, 160)
	switch w.Status = strings.ToLower(strings.TrimSpace(w.Status)); w.Status {
	case "", "ok", "warn", "bad", "info":
	case "good", "up", "pass", "passed", "success", "green":
		w.Status = "ok"
	case "error", "down", "fail", "failed", "critical", "red":
		w.Status = "bad"
	case "warning", "degraded", "amber", "yellow":
		w.Status = "warn"
	default:
		return w, fmt.Errorf("%w: status is ok, warn, bad or info", ErrBadWidget)
	}
	if w.Label == "" {
		w.Label = w.ID
	}
	if w.Value == "" && w.Text == "" {
		return w, fmt.Errorf("%w: give a value or text", ErrBadWidget)
	}
	w.TTL = max(0, min(w.TTL, 7*86400))
	w.Updated = time.Now()
	e.wmu.Lock()
	e.loadWidgetsLocked()
	if _, ok := e.widgets[w.ID]; !ok && len(e.widgets) >= maxWidgets {
		e.wmu.Unlock()
		return w, fmt.Errorf("%w: the board holds %d widgets; delete one first", ErrBadWidget, maxWidgets)
	}
	e.widgets[w.ID] = w
	err := e.saveWidgetsLocked()
	e.wmu.Unlock()
	if w.Flash {
		msg := w.Label + ": " + w.Value
		if w.Text != "" {
			msg = w.Label + ": " + w.Text
		}
		c := led.Alert
		switch w.Status {
		case "ok":
			c, _ = led.ParseColour("#46ff78")
		case "warn":
			c, _ = led.ParseColour("#ffd63c")
		}
		e.Flash(msg, 10, c)
	}
	return w, err
}

// DeleteWidget removes one widget, or all of them for id "*".
func (e *Engine) DeleteWidget(id string) error {
	e.wmu.Lock()
	defer e.wmu.Unlock()
	e.loadWidgetsLocked()
	if id == "*" {
		e.widgets = map[string]Widget{}
		return e.saveWidgetsLocked()
	}
	if _, ok := e.widgets[id]; !ok {
		return fmt.Errorf("%w: no widget %q", ErrBadWidget, id)
	}
	delete(e.widgets, id)
	return e.saveWidgetsLocked()
}

// Widgets lists the board, oldest first, without expired ones.
func (e *Engine) Widgets() []Widget {
	e.wmu.Lock()
	defer e.wmu.Unlock()
	e.loadWidgetsLocked()
	out := make([]Widget, 0, len(e.widgets))
	for id, w := range e.widgets {
		if w.TTL > 0 && time.Since(w.Updated).Seconds() > w.TTL {
			delete(e.widgets, id)
			continue
		}
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (e *Engine) loadWidgetsLocked() {
	if e.widgets != nil {
		return
	}
	e.widgets = map[string]Widget{}
	b, err := os.ReadFile(e.widgetsFile())
	if err != nil {
		return
	}
	var ws []Widget
	if json.Unmarshal(b, &ws) == nil {
		for _, w := range ws {
			if widgetID.MatchString(w.ID) {
				e.widgets[w.ID] = w
			}
		}
	}
}

func (e *Engine) saveWidgetsLocked() error {
	ws := make([]Widget, 0, len(e.widgets))
	for _, w := range e.widgets {
		ws = append(ws, w)
	}
	sort.Slice(ws, func(i, j int) bool { return ws[i].ID < ws[j].ID })
	b, _ := json.MarshalIndent(ws, "", "  ")
	return os.WriteFile(e.widgetsFile(), b, 0o644)
}

func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	return clip(strings.Trim(b.String(), "-"), 40)
}

// widgetViews converts the board for the screen.
func (e *Engine) widgetViews() []dashboard.WidgetView {
	var out []dashboard.WidgetView
	for _, w := range e.Widgets() {
		out = append(out, dashboard.WidgetView{Label: w.Label, Value: w.Value, Unit: w.Unit, Status: w.Status, Text: w.Text, Updated: w.Updated})
	}
	return out
}

// ---------------------------------------------------------------- relay

// The relay lets tools that cannot reach this Mac (a cloud agent such as Grok Bot, a CI runner)
// post widgets: they publish to an ntfy topic and Marquee polls it. The topic name is the only
// secret, so it is kept in relay-topic (0600). A message that is JSON is a widget (or a list of
// them); plain text becomes a widget named after the message title.

// RelayFile holds the relay topic URL.
func (e *Engine) RelayFile() string { return filepath.Join(e.paths.Dir, "relay-topic") }

func (e *Engine) relayURL() string {
	b, err := os.ReadFile(e.RelayFile())
	if err != nil {
		return ""
	}
	u, err := ParseRelay(string(b))
	if err != nil {
		return ""
	}
	return u
}

var ntfyTopic = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// ParseRelay turns "ntfy.sh/topic", "https://ntfy.sh/topic" or a bare topic into a URL.
func ParseRelay(v string) (string, error) {
	v = strings.TrimSpace(v)
	if !strings.Contains(v, "/") {
		v = "https://ntfy.sh/" + v
	}
	if !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
		v = "https://" + v
	}
	u, err := url.Parse(v)
	if err != nil || u.Host == "" {
		return "", errors.New("give an ntfy topic, e.g. ntfy.sh/marquee-a7f3k9q2x")
	}
	topic := strings.Trim(u.Path, "/")
	if !ntfyTopic.MatchString(topic) {
		return "", errors.New("the topic is 8-64 letters, digits, - or _; make it long and random, as anyone who knows it can post")
	}
	return u.Scheme + "://" + u.Host + "/" + topic, nil
}

// relayLoop polls the relay every 20 seconds while one is set.
func (e *Engine) relayLoop() {
	since := "5m"
	tick := time.NewTicker(20 * time.Second)
	defer tick.Stop()
	for {
		if u := e.relayURL(); u != "" {
			if next, err := e.pollRelay(u, since); err == nil {
				since = next
			} else {
				e.log.Debug("relay", "err", err)
			}
		}
		select {
		case <-e.stop:
			return
		case <-tick.C:
		}
	}
}

func (e *Engine) pollRelay(u, since string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, u+"/json?poll=1&since="+url.QueryEscape(since), nil)
	if err != nil {
		return since, err
	}
	req.Header.Set("User-Agent", "Marquee")
	c := &http.Client{Timeout: 15 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return since, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return since, fmt.Errorf("relay: HTTP %d", resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var m struct {
			ID, Event, Title, Message string
		}
		if json.Unmarshal(sc.Bytes(), &m) != nil || m.Event != "message" {
			continue
		}
		since = m.ID
		for _, w := range relayWidgets(m.Title, m.Message) {
			if _, err := e.SetWidget(w); err != nil {
				e.log.Debug("relay widget", "err", err)
			}
		}
	}
	return since, sc.Err()
}

func relayWidgets(title, msg string) []Widget {
	msg = strings.TrimSpace(msg)
	if strings.HasPrefix(msg, "[") {
		var ws []Widget
		if json.Unmarshal([]byte(msg), &ws) == nil {
			return ws
		}
	}
	if strings.HasPrefix(msg, "{") {
		var w Widget
		if json.Unmarshal([]byte(msg), &w) == nil {
			return []Widget{w}
		}
	}
	if title == "" {
		title = "Relay"
	}
	return []Widget{{ID: slug(title), Label: title, Text: msg, Status: "info"}}
}
