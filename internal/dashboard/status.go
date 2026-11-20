package dashboard

import (
	"fmt"
	"image"
	"image/color"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// Status watches cloud services' status pages: one light per service, and the open incidents
// for any that are not fully up.
type Status struct {
	pages func() []feeds.StatusPage
	fetch func(feeds.StatusPage) (*feeds.ServiceStatus, error)
	poll  poller[[]stResult]
	cache frameCache
}

type stResult struct {
	page feeds.StatusPage
	s    *feeds.ServiceStatus // nil when the page did not answer
}

// DefaultStatus is what status.txt holds when there is none.
var DefaultStatus = []string{"github", "cloudflare", "anthropic", "openai", "atlassian", "vercel"}

const stMax = 12

func NewStatus(pages func() []feeds.StatusPage) *Status {
	return &Status{pages: pages, fetch: feeds.FetchStatus, poll: poller[[]stResult]{every: 2 * time.Minute}}
}

// ParseStatus reads status.txt: one service per line, a preset key (github, cloudflare...) or
// `Name = https://status.example.com` for any Atlassian Statuspage. # starts a comment.
func ParseStatus(lines []string) (pages []feeds.StatusPage, bad []string) {
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if name, u, ok := strings.Cut(l, "="); ok {
			name, u = strings.TrimSpace(name), strings.TrimSpace(u)
			if !strings.Contains(u, "://") {
				u = "https://" + u
			}
			pu, err := url.Parse(u)
			if name == "" || err != nil || pu.Host == "" || (pu.Scheme != "https" && pu.Scheme != "http") {
				bad = append(bad, l)
				continue
			}
			pages = append(pages, feeds.StatusPage{Name: name, URL: strings.TrimRight(pu.Scheme+"://"+pu.Host+pu.Path, "/")})
		} else if p, ok := feeds.StatusPresets[strings.ToLower(l)]; ok {
			pages = append(pages, p)
		} else {
			bad = append(bad, l)
			continue
		}
		if len(pages) == stMax {
			break
		}
	}
	return pages, bad
}

// LoadStatus reads status.txt, falling back to DefaultStatus.
func LoadStatus(path string) []feeds.StatusPage {
	if b, err := os.ReadFile(path); err == nil {
		if p, _ := ParseStatus(strings.Split(string(b), "\n")); len(p) > 0 {
			return p
		}
	}
	p, _ := ParseStatus(DefaultStatus)
	return p
}

func (st *Status) load(pages []feeds.StatusPage) ([]stResult, error) {
	out := make([]stResult, len(pages))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for i, p := range pages {
		out[i].page = p
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := st.fetch(p)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			out[i].s = s
		}()
	}
	wg.Wait()
	for _, r := range out {
		if r.s != nil {
			return out, nil
		}
	}
	if firstErr == nil {
		firstErr = fmt.Errorf("no services in status.txt")
	}
	return nil, firstErr
}

func (st *Status) Frame(t, dt float64, now time.Time) *image.RGBA {
	pages := st.pages()
	var key strings.Builder
	for _, p := range pages {
		key.WriteString(p.URL + "|")
	}
	rs, got, ok, err := st.poll.get(key.String(), func(string) ([]stResult, error) { return st.load(pages) })
	if !ok {
		return waitMessage(&st.cache, "SERVICE STATUS", err)
	}
	return st.cache.get(fmt.Sprint(got.UnixNano()), func(img *image.RGBA) { stRender(img, rs, got.In(now.Location())) })
}

// stColour is a status indicator's colour and a word for it.
func stColour(s *feeds.ServiceStatus) (color.RGBA, string) {
	if s == nil {
		return txt3, "NO RESPONSE"
	}
	switch s.Indicator {
	case "none":
		if len(s.Down) > 0 {
			return amber, "DEGRADED"
		}
		return green, "OPERATIONAL"
	case "minor":
		return amber, "MINOR"
	case "major":
		return hot, "MAJOR"
	case "critical":
		return hot, "CRITICAL"
	case "maintenance":
		return violet, "MAINTENANCE"
	}
	return txt3, strings.ToUpper(s.Indicator)
}

const (
	stX1 = 560  // services grid
	stX2 = 1260 // incidents
)

func stRender(img *image.RGBA, rs []stResult, got time.Time) {
	head(img, pad, pad-4, "service status", stX1-pad-40)
	issues, answered := 0, 0
	worst := green
	for _, r := range rs {
		c, _ := stColour(r.s)
		if r.s != nil {
			answered++
		}
		if c == amber || c == hot || c == violet {
			issues++
			if worst != hot {
				worst = c
			}
		}
	}
	say := fmt.Sprintf("ALL %d UP", answered)
	if issues > 0 {
		say = fmt.Sprintf("%d ISSUE", issues)
		if issues > 1 {
			say += "S"
		}
	}
	text(img, pad, 84, say, fitPx(say, 96, stX1-pad-60, 110), worst)
	text(img, pad, 214, fmt.Sprintf("%d SERVICES WATCHED", len(rs)), 24, txt)
	if n := len(rs) - answered; n > 0 {
		text(img, pad, 254, fmt.Sprintf("%d NOT ANSWERING", n), 24, txt3)
	}
	text(img, pad, 370, "CHECKED "+got.Format("15:04"), 22, txt2)
	text(img, pad, 410, "STATUS PAGES, EVERY 2 MINUTES", 16, txt3)
	vrule(img, stX1-30)

	// one light per service, two columns
	x := stX1 + 10
	head(img, x, pad-4, "services", stX2-x-40)
	rows := (len(rs) + 1) / 2
	rowH := min(56, (H-pad-80)/max(rows, 1))
	colW := (stX2 - 40 - x) / 2
	for i, r := range rs {
		cx := x + (i/rows)*colW
		cy := 80 + (i%rows)*rowH
		c, word := stColour(r.s)
		disc(img, float64(cx+12), float64(cy+14), 10, c)
		nw := float64(colW - 56)
		if c != green {
			nw -= measure(word, 18) + 16
			text(img, float64(cx+colW-20)-measure(word, 18), cy+6, word, 18, c)
		}
		text(img, float64(cx+36), cy, fit(strings.ToUpper(r.page.Name), 26, nw), 26, txt)
	}
	vrule(img, stX2-30)

	// what is wrong
	x = stX2 + 10
	head(img, x, pad-4, "open incidents", W-pad-x)
	y := 80
	w := float64(W - pad - x)
	any := false
	for _, r := range rs {
		if r.s == nil || y > H-pad-40 {
			continue
		}
		c, _ := stColour(r.s)
		for _, in := range r.s.Incidents {
			if y > H-pad-40 {
				break
			}
			any = true
			ic := amber
			if in.Impact == "major" || in.Impact == "critical" {
				ic = hot
			}
			text(img, float64(x), y, fit(strings.ToUpper(r.page.Name+"  "+in.Status), 18, w), 18, ic)
			y += 24
			for _, l := range wrap(in.Name, 22, w, 2) {
				text(img, float64(x), y, l, 22, txt)
				y += 28
			}
			if in.Update != "" {
				text(img, float64(x), y, fit(in.Update, 18, w), 18, txt3)
				y += 26
			}
			y += 10
		}
		if len(r.s.Incidents) == 0 && len(r.s.Down) > 0 {
			any = true
			text(img, float64(x), y, strings.ToUpper(r.page.Name), 18, c)
			y += 24
			var names []string
			for _, d := range r.s.Down {
				names = append(names, d.Name+" ("+strings.ReplaceAll(d.Status, "_", " ")+")")
			}
			for _, l := range wrap(strings.Join(names, ", "), 20, w, 2) {
				text(img, float64(x), y, l, 20, txt2)
				y += 26
			}
			y += 10
		}
	}
	if !any {
		text(img, float64(x), 84, "NONE", 40, green)
		text(img, float64(x), 140, "EVERYTHING WATCHED IS RUNNING NORMALLY", 18, txt3)
	}
}
