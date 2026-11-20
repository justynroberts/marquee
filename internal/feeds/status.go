package feeds

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// StatusPage is a service's public status page (Atlassian Statuspage, which most use).
type StatusPage struct{ Name, URL string }

// StatusPresets are status pages known by a short key in status.txt.
var StatusPresets = map[string]StatusPage{
	"github":       {"GitHub", "https://www.githubstatus.com"},
	"cloudflare":   {"Cloudflare", "https://www.cloudflarestatus.com"},
	"anthropic":    {"Anthropic", "https://status.claude.com"},
	"claude":       {"Claude", "https://status.claude.com"},
	"openai":       {"OpenAI", "https://status.openai.com"},
	"atlassian":    {"Atlassian", "https://status.atlassian.com"},
	"zoom":         {"Zoom", "https://www.zoomstatus.com"},
	"discord":      {"Discord", "https://discordstatus.com"},
	"digitalocean": {"DigitalOcean", "https://status.digitalocean.com"},
	"vercel":       {"Vercel", "https://www.vercel-status.com"},
	"netlify":      {"Netlify", "https://www.netlifystatus.com"},
	"npm":          {"npm", "https://status.npmjs.org"},
}

// StatusPresetKeys lists the preset keys in order.
func StatusPresetKeys() []string {
	keys := make([]string, 0, len(StatusPresets))
	for k := range StatusPresets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ServiceStatus is one status page now.
type ServiceStatus struct {
	Name        string
	Indicator   string // none, minor, major, critical, maintenance
	Description string // "All Systems Operational"
	Down        []Component
	Incidents   []StatusIncident
}

// Component is a part of a service that is not operational.
type Component struct{ Name, Status string } // degraded_performance, partial_outage, major_outage, under_maintenance

// StatusIncident is an unresolved incident with its latest update.
type StatusIncident struct {
	Name, Impact, Status, Update string
	Updated                      time.Time
}

// FetchStatus reads a status page's /api/v2/summary.json.
func FetchStatus(p StatusPage) (*ServiceStatus, error) {
	var r struct {
		Status struct {
			Indicator   string `json:"indicator"`
			Description string `json:"description"`
		} `json:"status"`
		Components []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Group  bool   `json:"group"`
		} `json:"components"`
		Incidents []struct {
			Name    string `json:"name"`
			Status  string `json:"status"`
			Impact  string `json:"impact"`
			Updates []struct {
				Body      string    `json:"body"`
				UpdatedAt time.Time `json:"updated_at"`
			} `json:"incident_updates"`
			UpdatedAt time.Time `json:"updated_at"`
		} `json:"incidents"`
	}
	if err := getJSON(strings.TrimRight(p.URL, "/")+"/api/v2/summary.json", &r); err != nil {
		return nil, fmt.Errorf("%s: %v", p.Name, err)
	}
	s := &ServiceStatus{Name: p.Name, Indicator: r.Status.Indicator, Description: r.Status.Description}
	for _, c := range r.Components {
		if c.Status != "operational" && c.Status != "" && !c.Group {
			s.Down = append(s.Down, Component{c.Name, c.Status})
		}
	}
	for _, in := range r.Incidents {
		if in.Status == "resolved" || in.Status == "postmortem" {
			continue
		}
		i := StatusIncident{Name: in.Name, Impact: in.Impact, Status: in.Status, Updated: in.UpdatedAt}
		if len(in.Updates) > 0 {
			i.Update = strings.Join(strings.Fields(in.Updates[0].Body), " ")
			i.Updated = in.Updates[0].UpdatedAt
		}
		s.Incidents = append(s.Incidents, i)
	}
	return s, nil
}
