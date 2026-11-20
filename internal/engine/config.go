package engine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/justynroberts/marquee/internal/dashboard"
	"github.com/justynroberts/marquee/internal/feeds"
	"github.com/justynroberts/marquee/internal/location"
)

// A Setting is one of the screens' text files in the data folder, as the settings page shows
// it. Saving checks the value first (looking it up where that is the only real check) and says
// what Marquee made of it.
type Setting struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Help     string   `json:"help"`
	File     string   `json:"file"`
	Kind     string   `json:"kind"` // line, lines or secret
	Example  string   `json:"example"`
	Programs []string `json:"programs"`
	Value    string   `json:"value"`   // "" for a secret
	Set      bool     `json:"set"`     // the file exists
	Current  string   `json:"current"` // what is in effect now, in words

	check func(e *Engine, v string) (string, error)
	now   func(e *Engine) string
}

var (
	crsRe     = regexp.MustCompile(`^[A-Z]{3}$`)
	outcodeRe = regexp.MustCompile(`^[A-Z]{1,2}[0-9][A-Z0-9]?$`)
)

func settingsList() []Setting {
	return []Setting{
		{
			ID: "location", Title: "Location", File: "weather.txt", Kind: "line",
			Help:     "Where the weather, rain, air quality, flights, sun, ISS, day and night, and grid screens are for: a UK postcode or a place name. Leave empty to use this Mac's location.",
			Example:  "SW1A 1AA",
			Programs: []string{"weather", "rain", "air", "flights", "sun", "iss", "daynight", "grid"},
			check: func(e *Engine, v string) (string, error) {
				p, err := feeds.Geocode(v)
				if err != nil {
					return "", err
				}
				s := fmt.Sprintf("%s  %.2f, %.2f", p.Name, p.Latitude, p.Longitude)
				if pc, ok := feeds.UKPostcode(v); ok {
					s += "  ·  grid region " + feeds.Outcode(pc)
				}
				return s, nil
			},
			now: func(e *Engine) string {
				if f, ok, err := location.Current(); ok {
					return "this Mac's location: " + f.Name
				} else if err != nil {
					return "London (" + err.Error() + ")"
				}
				return "this Mac's location, once macOS has found it"
			},
		},
		{
			ID: "worldclock", Title: "World clock", File: "world-clock.txt", Kind: "lines",
			Help:     "One city per line, up to eight: Label = Area/City, or just a time zone name.",
			Example:  "Home = Europe/London\nNew York = America/New_York\nAsia/Tokyo",
			Programs: []string{"worldclock"},
			check: func(e *Engine, v string) (string, error) {
				z, bad := dashboard.ParseZones(strings.Split(v, "\n"))
				if len(bad) > 0 {
					return "", fmt.Errorf("not a time zone: %s", strings.Join(bad, ", "))
				}
				var names []string
				for _, c := range z {
					names = append(names, c.Label)
				}
				return strings.Join(names, ", "), nil
			},
			now: func(e *Engine) string { return "San Francisco, New York, London, Tokyo, Sydney" },
		},
		{
			ID: "newsreel", Title: "Newsreel", File: "newsreel.txt", Kind: "line",
			Help:     "The source the newsreel crawls: bbc, bbc-world, bbc-tech, bbc-sport, sky, guardian, aljazeera, npr, hn, verge, ars, or any RSS or Atom feed URL.",
			Example:  "sky",
			Programs: []string{"newsreel"},
			check: func(e *Engine, v string) (string, error) {
				s, ok := feeds.LookupSource(v)
				if !ok {
					return "", errors.New("not a preset name or a feed URL")
				}
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				hs, err := s.Fetch(ctx, 3)
				if err != nil {
					return "", err
				}
				label := s.Label
				if label == "" && len(hs) > 0 {
					label = hs[0].Source
				}
				return fmt.Sprintf("%s  ·  latest: %s", label, hs[0].Title), nil
			},
			now: func(e *Engine) string { return "BBC News" },
		},
		{
			ID: "news", Title: "News feeds", File: "news-sources.txt", Kind: "lines",
			Help:     "Sources for the news ticker and the headlines wall, one per line: hn for Hacker News, or an RSS or Atom feed URL.",
			Example:  "hn\nhttps://feeds.bbci.co.uk/news/rss.xml",
			Programs: []string{"news", "headlines"},
			check: func(e *Engine, v string) (string, error) {
				n := 0
				for _, l := range strings.Split(v, "\n") {
					l = strings.TrimSpace(l)
					if l == "" || strings.HasPrefix(l, "#") {
						continue
					}
					if !strings.EqualFold(l, "hn") && !strings.EqualFold(l, "hackernews") && !strings.HasPrefix(l, "http://") && !strings.HasPrefix(l, "https://") {
						return "", fmt.Errorf("not hn or a feed URL: %s", l)
					}
					n++
				}
				go e.RefreshNews()
				return fmt.Sprintf("%d sources; fetching now", n), nil
			},
			now: func(e *Engine) string { return "Hacker News (and BBC News on the headlines wall)" },
		},
		{
			ID: "markets", Title: "Coins", File: "markets.txt", Kind: "lines",
			Help:     "Up to four CoinGecko coin ids (the name in the coin page's address), one per line, and optionally currency = gbp.",
			Example:  "bitcoin\nethereum\ncurrency = gbp",
			Programs: []string{"markets"},
			check: func(e *Engine, v string) (string, error) {
				c := dashboard.ParseMarkets(v)
				coins, err := feeds.FetchCoins(c.Coins, c.Currency)
				if err != nil {
					return "", err
				}
				var got []string
				found := map[string]bool{}
				for _, cn := range coins {
					got = append(got, fmt.Sprintf("%s %s", cn.Symbol, cn.Name))
					found[cn.ID] = true
				}
				for _, id := range c.Coins {
					if !found[id] {
						return "", fmt.Errorf("CoinGecko has no coin %q", id)
					}
				}
				return strings.Join(got, ", ") + " in " + strings.ToUpper(c.Currency), nil
			},
			now: func(e *Engine) string { return "Bitcoin, Ethereum, Solana in USD" },
		},
		{
			ID: "countdown", Title: "Countdown", File: "countdown.txt", Kind: "lines",
			Help:     "One per line: Label = YYYY-MM-DD, or Label = MM-DD for every year, with an optional time.",
			Example:  "Holiday = 2027-07-14\nLaunch = 2027-02-01 09:00\nBirthday = 03-14",
			Programs: []string{"countdown"},
			check: func(e *Engine, v string) (string, error) {
				evs, bad := dashboard.ParseCountdown(strings.Split(v, "\n"))
				if len(bad) > 0 {
					return "", fmt.Errorf("cannot read: %s", strings.Join(bad, ", "))
				}
				return fmt.Sprintf("%d dates", len(evs)), nil
			},
			now: func(e *Engine) string { return "Christmas and New Year" },
		},
		{
			ID: "calendar", Title: "Bank holidays", File: "calendar.txt", Kind: "line",
			Help:     "Which UK bank holidays the calendar marks: england-and-wales, scotland, northern-ireland, or none.",
			Example:  "scotland",
			Programs: []string{"calendar"},
			check: func(e *Engine, v string) (string, error) {
				v = strings.ToLower(v)
				if v == "none" {
					return "no bank holidays", nil
				}
				for _, a := range feeds.HolidayAreas {
					if v == a {
						hs, err := feeds.FetchHolidays(a)
						if err != nil {
							return "", err
						}
						return fmt.Sprintf("%d bank holidays", len(hs)), nil
					}
				}
				return "", fmt.Errorf("use %s or none", strings.Join(feeds.HolidayAreas, ", "))
			},
			now: func(e *Engine) string { return "England and Wales" },
		},
		{
			ID: "trains", Title: "Station", File: "trains.txt", Kind: "line",
			Help:     "The station's three-letter code: LGE Long Eaton, NOT Nottingham, DBY Derby, EMD East Midlands Parkway.",
			Example:  "NOT",
			Programs: []string{"trains"},
			check: func(e *Engine, v string) (string, error) {
				v = strings.ToUpper(v)
				if !crsRe.MatchString(v) {
					return "", errors.New("a station code is three letters")
				}
				if key := e.trainToken(); key != "" {
					b, err := feeds.FetchBoard(key, v, time.Hour)
					if err != nil {
						return "", err
					}
					return fmt.Sprintf("%s  ·  %d departures in the next hour", b.Station, len(b.Departures)), nil
				}
				return v + " (add a train times key to check it)", nil
			},
			now: func(e *Engine) string { return "LGE, Long Eaton" },
		},
		{
			ID: "railkey", Title: "Train times key", File: "rail-key", Kind: "secret",
			Help:     "Free: register at raildata.org.uk, subscribe to Live Arrival and Departure Boards, and paste the Consumer key from its Specification tab. It stays on this Mac.",
			Programs: []string{"trains"},
			check: func(e *Engine, v string) (string, error) {
				b, err := feeds.FetchBoard(v, e.trainStation(), time.Hour)
				if err != nil {
					return "", err
				}
				return "works: " + b.Station, nil
			},
			now: func(e *Engine) string { return "not set" },
		},
		{
			ID: "status", Title: "Status pages", File: "status.txt", Kind: "lines",
			Help:     "Services to watch, one per line: a preset (" + strings.Join(feeds.StatusPresetKeys(), ", ") + "), or Name = the address of any Atlassian Statuspage site.",
			Example:  "github\ncloudflare\nMy app = https://status.example.com",
			Programs: []string{"status"},
			check: func(e *Engine, v string) (string, error) {
				pages, bad := dashboard.ParseStatus(strings.Split(v, "\n"))
				if len(bad) > 0 {
					return "", fmt.Errorf("not a preset or Name = URL: %s", strings.Join(bad, ", "))
				}
				if len(pages) == 0 {
					return "", errors.New("no services")
				}
				var got []string
				for _, p := range pages {
					st, err := feeds.FetchStatus(p)
					if err != nil {
						return "", fmt.Errorf("%s: %v", p.Name, err)
					}
					got = append(got, p.Name+" "+strings.ToLower(st.Description))
				}
				return strings.Join(got, ", "), nil
			},
			now: func(e *Engine) string {
				var names []string
				for _, p := range dashboard.LoadStatus("") {
					names = append(names, p.Name)
				}
				return strings.Join(names, ", ")
			},
		},
		{
			ID: "fx", Title: "Currencies", File: "fx.txt", Kind: "line",
			Help:     "The base currency, a colon, then up to six to price in it, as three-letter codes.",
			Example:  "GBP: USD EUR JPY",
			Programs: []string{"fx"},
			check: func(e *Engine, v string) (string, error) {
				c, err := dashboard.ParseFX([]string{v})
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("1 %s in %s", c.Base, strings.Join(c.Quotes, ", ")), nil
			},
			now: func(e *Engine) string {
				c := dashboard.LoadFX("")
				return fmt.Sprintf("1 %s in %s", c.Base, strings.Join(c.Quotes, ", "))
			},
		},
		{
			ID: "focus", Title: "Focus timer", File: "focus.txt", Kind: "line",
			Help:     "Minutes of focus, then break, then optionally the long break and how many sessions come before it.",
			Example:  "50 10 30 3",
			Programs: []string{"pomodoro"},
			check: func(e *Engine, v string) (string, error) {
				c, err := dashboard.ParseFocus(v)
				if err != nil {
					return "", err
				}
				return focusSummary(c), nil
			},
			now: func(e *Engine) string { return focusSummary(dashboard.LoadFocus("")) },
		},
		{
			ID: "relay", Title: "Widget relay", File: "relay-topic", Kind: "secret",
			Help:     "Lets cloud tools (Grok Bot, CI, a server) post widgets: they send the widget JSON to this ntfy.sh topic and Marquee checks it every 20 seconds. Pick a long random topic name; anyone who knows it can post.",
			Programs: []string{"widgets"},
			check: func(e *Engine, v string) (string, error) {
				u, err := ParseRelay(v)
				if err != nil {
					return "", err
				}
				c := &http.Client{Timeout: 10 * time.Second}
				resp, err := c.Get(u + "/json?poll=1&since=1m")
				if err != nil {
					return "", err
				}
				resp.Body.Close()
				if resp.StatusCode != 200 {
					return "", fmt.Errorf("the relay answered HTTP %d", resp.StatusCode)
				}
				return "listening on " + u, nil
			},
			now: func(e *Engine) string { return "off" },
		},
		{
			ID: "grid", Title: "Grid region", File: "grid.txt", Kind: "line",
			Help:     "A postcode district such as SW1A, or gb for the whole country. Leave empty to follow your location.",
			Example:  "SW1A",
			Programs: []string{"grid"},
			check: func(e *Engine, v string) (string, error) {
				v = strings.ToUpper(v)
				if v == "GB" || v == "NATIONAL" {
					v = ""
				} else if !outcodeRe.MatchString(v) {
					return "", errors.New("a postcode district looks like SW1A, or use gb")
				}
				g, err := feeds.FetchGrid(v)
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("%s  ·  %d gCO2/kWh now", g.Region, g.Intensity), nil
			},
			now: func(e *Engine) string {
				if oc := e.gridOutcode(); oc != "" && !strings.HasPrefix(oc, "@") {
					return oc + ", from your location"
				}
				return "from your location"
			},
		},
	}
}

func focusSummary(c dashboard.FocusConfig) string {
	return fmt.Sprintf("%.0f min focus, %.0f min break, %.0f min long break every %d", c.Work.Minutes(), c.Break.Minutes(), c.Long.Minutes(), c.Every)
}

// Settings lists the screens' settings with their current values.
func (e *Engine) Settings() []Setting {
	out := settingsList()
	for i := range out {
		s := &out[i]
		b, err := os.ReadFile(filepath.Join(e.paths.Dir, s.File))
		s.Set = err == nil
		switch s.Kind {
		case "lines":
			s.Value = strings.TrimRight(string(b), "\n")
		case "line": // the first line that is not a comment
			for _, l := range strings.Split(string(b), "\n") {
				if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
					s.Value = l
					break
				}
			}
		}
		if !s.Set && s.now != nil {
			s.Current = s.now(e)
		}
	}
	return out
}

// ErrUnknownSetting is returned for an id not in Settings.
var ErrUnknownSetting = errors.New("no such setting")

// SetSetting checks and saves one setting; an empty value removes the file (back to the
// default). It returns what Marquee made of the value.
func (e *Engine) SetSetting(id, value string) (string, error) {
	for _, s := range settingsList() {
		if s.ID != id {
			continue
		}
		path := filepath.Join(e.paths.Dir, s.File)
		value = strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
		if s.Kind != "lines" {
			value = strings.TrimSpace(strings.SplitN(value, "\n", 2)[0])
		}
		if value == "" {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
			if s.now != nil {
				return "default: " + s.now(e), nil
			}
			return "default", nil
		}
		summary, err := s.check(e, value)
		if err != nil {
			return "", err
		}
		mode := os.FileMode(0o644)
		if s.Kind == "secret" {
			mode = 0o600
		}
		if err := os.WriteFile(path, []byte(value+"\n"), mode); err != nil {
			return "", err
		}
		_ = os.Chmod(path, mode)
		e.reloadCurrent(s.Programs)
		return summary, nil
	}
	return "", ErrUnknownSetting
}

// reloadCurrent restarts the playing program when it reads its settings once at the start, so
// a change shows at once.
func (e *Engine) reloadCurrent(progs []string) {
	e.mu.Lock()
	cur := e.state.Program
	e.mu.Unlock()
	for _, p := range progs {
		if p == cur {
			_ = e.Play(cur)
			return
		}
	}
}
