package feeds

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Claude plan limits: how much of the five-hour session and the weekly allowance a Pro or Max
// subscription has used, the same figures Claude Code's /usage shows. They come from the usage
// endpoint Claude Code itself calls, authorised with the login Claude Code keeps in the
// Keychain. The token is read for each fetch, sent only to api.anthropic.com and never stored
// or refreshed here (refreshing would sign Claude Code out); Claude Code keeps it fresh.

// ClaudeLimit is one allowance.
type ClaudeLimit struct {
	Name    string    // "SESSION", "WEEK", or a model for a scoped weekly limit
	Used    float64   // percent used, 0-100
	Resets  time.Time // zero if unknown
	Window  time.Duration
	Warning bool // the service flags it as running short
}

// Left is the percent still available.
func (l ClaudeLimit) Left() float64 { return max(0, 100-l.Used) }

// ClaudeShare is one app's share of the week's use.
type ClaudeShare struct {
	Name    string
	Percent float64
}

// ClaudeLimits is a snapshot of the plan's allowances.
type ClaudeLimits struct {
	Plan    string // "MAX 5X", "PRO"
	Session ClaudeLimit
	Week    ClaudeLimit
	Scoped  []ClaudeLimit // per-model weekly limits
	Shares  []ClaudeShare // the week by app: Claude Code, chats, Cowork
	Extra   bool          // extra usage (paid overage) switched on
}

// ErrClaudeLogin means there is no usable Claude Code login on this Mac.
var ErrClaudeLogin = errors.New("open Claude Code and sign in to see plan limits")

type ccCreds struct {
	OAuth struct {
		AccessToken      string `json:"accessToken"`
		ExpiresAt        int64  `json:"expiresAt"`
		SubscriptionType string `json:"subscriptionType"`
		RateLimitTier    string `json:"rateLimitTier"`
	} `json:"claudeAiOauth"`
}

// claudeCreds reads Claude Code's login: the Keychain on a Mac, else ~/.claude/.credentials.json.
var claudeCreds = func() ([]byte, error) {
	if b, err := exec.Command("/usr/bin/security", "find-generic-password", "-s", "Claude Code-credentials", "-w").Output(); err == nil {
		return b, nil
	}
	return os.ReadFile(filepath.Join(ClaudeHome(), ".credentials.json"))
}

// ClaudeUsageURL is the endpoint Claude Code's /usage reads.
var ClaudeUsageURL = "https://api.anthropic.com/api/oauth/usage"

// FetchClaudeLimits asks Anthropic how much of the plan is used.
func FetchClaudeLimits() (*ClaudeLimits, error) {
	raw, err := claudeCreds()
	if err != nil {
		return nil, ErrClaudeLogin
	}
	var c ccCreds
	if json.Unmarshal(raw, &c) != nil || c.OAuth.AccessToken == "" {
		return nil, ErrClaudeLogin
	}
	if c.OAuth.ExpiresAt > 0 && time.Now().UnixMilli() > c.OAuth.ExpiresAt {
		return nil, errors.New("Claude Code login has expired; open Claude Code to renew it")
	}
	req, _ := http.NewRequest(http.MethodGet, ClaudeUsageURL, nil)
	req.Header.Set("Authorization", "Bearer "+c.OAuth.AccessToken)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("User-Agent", "Marquee")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return nil, ErrClaudeLogin
	case resp.StatusCode != 200:
		return nil, fmt.Errorf("plan limits: HTTP %d", resp.StatusCode)
	}
	var body json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	l, err := ParseClaudeLimits(body)
	if err != nil {
		return nil, err
	}
	l.Plan = PlanName(c.OAuth.SubscriptionType, c.OAuth.RateLimitTier)
	return l, nil
}

// PlanName turns the subscription type and tier into "MAX 5X", "MAX 20X" or "PRO".
func PlanName(sub, tier string) string {
	p := strings.ToUpper(sub)
	if p == "" {
		p = "CLAUDE"
	}
	for _, m := range []string{"20x", "5x"} {
		if strings.HasSuffix(tier, "_"+m) {
			return p + " " + strings.ToUpper(m)
		}
	}
	return p
}

type limWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    string   `json:"resets_at"`
}

// ParseClaudeLimits reads the usage endpoint's answer.
func ParseClaudeLimits(b []byte) (*ClaudeLimits, error) {
	var r struct {
		FiveHour *limWindow `json:"five_hour"`
		SevenDay *limWindow `json:"seven_day"`
		Limits   []struct {
			Kind     string  `json:"kind"`
			Percent  float64 `json:"percent"`
			Severity string  `json:"severity"`
			ResetsAt string  `json:"resets_at"`
			Scope    *struct {
				Model *struct {
					DisplayName string `json:"display_name"`
				} `json:"model"`
			} `json:"scope"`
		} `json:"limits"`
		Breakdown *struct {
			Rows []struct {
				Name    string  `json:"display_name"`
				Percent float64 `json:"percent"`
			} `json:"rows"`
		} `json:"seven_day_breakdown"`
		Extra *struct {
			Enabled bool `json:"is_enabled"`
		} `json:"extra_usage"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	if r.FiveHour == nil && r.SevenDay == nil && len(r.Limits) == 0 {
		return nil, errors.New("plan limits: no limits in the answer")
	}
	when := func(s string) time.Time {
		t, _ := time.Parse(time.RFC3339Nano, s)
		return t
	}
	win := func(name string, w *limWindow, span time.Duration) ClaudeLimit {
		l := ClaudeLimit{Name: name, Window: span}
		if w != nil {
			if w.Utilization != nil {
				l.Used = *w.Utilization
			}
			l.Resets = when(w.ResetsAt)
		}
		return l
	}
	out := &ClaudeLimits{
		Session: win("SESSION", r.FiveHour, 5*time.Hour),
		Week:    win("WEEK", r.SevenDay, 7*24*time.Hour),
	}
	for _, x := range r.Limits {
		warn := x.Severity != "" && x.Severity != "normal"
		switch x.Kind {
		case "session":
			out.Session.Used, out.Session.Warning = x.Percent, warn
			if t := when(x.ResetsAt); !t.IsZero() {
				out.Session.Resets = t
			}
		case "weekly_all":
			out.Week.Used, out.Week.Warning = x.Percent, warn
			if t := when(x.ResetsAt); !t.IsZero() {
				out.Week.Resets = t
			}
		case "weekly_scoped":
			name := "MODEL"
			if x.Scope != nil && x.Scope.Model != nil && x.Scope.Model.DisplayName != "" {
				name = strings.ToUpper(x.Scope.Model.DisplayName)
			}
			out.Scoped = append(out.Scoped, ClaudeLimit{Name: name, Used: x.Percent, Resets: when(x.ResetsAt), Window: 7 * 24 * time.Hour, Warning: warn})
		}
	}
	if r.Breakdown != nil {
		for _, row := range r.Breakdown.Rows {
			if row.Percent > 0 {
				out.Shares = append(out.Shares, ClaudeShare{Name: row.Name, Percent: row.Percent})
			}
		}
	}
	out.Extra = r.Extra != nil && r.Extra.Enabled
	return out, nil
}
