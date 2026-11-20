package feeds

import "testing"

func TestParseClaudeLimits(t *testing.T) {
	b := []byte(`{"five_hour":{"utilization":82.0,"resets_at":"2026-11-13T11:30:00.1+00:00"},
	"seven_day":{"utilization":35.0,"resets_at":"2026-11-15T22:00:00+00:00"},
	"limits":[{"kind":"session","percent":82,"severity":"warning","resets_at":"2026-11-13T11:30:00.1+00:00"},
	{"kind":"weekly_all","percent":35,"severity":"normal","resets_at":"2026-11-15T22:00:00+00:00"},
	{"kind":"weekly_scoped","percent":4,"severity":"normal","resets_at":"2026-11-15T22:00:00+00:00","scope":{"model":{"display_name":"Fable"}}}],
	"seven_day_breakdown":{"rows":[{"display_name":"Claude Code","percent":89},{"display_name":"Other","percent":0}]},
	"extra_usage":{"is_enabled":false}}`)
	l, err := ParseClaudeLimits(b)
	if err != nil {
		t.Fatal(err)
	}
	if l.Session.Left() != 18 || !l.Session.Warning || l.Session.Resets.Hour() != 11 {
		t.Errorf("session %+v", l.Session)
	}
	if l.Week.Used != 35 || l.Week.Warning || l.Week.Resets.Day() != 15 {
		t.Errorf("week %+v", l.Week)
	}
	if len(l.Scoped) != 1 || l.Scoped[0].Name != "FABLE" || l.Scoped[0].Used != 4 {
		t.Errorf("scoped %+v", l.Scoped)
	}
	if len(l.Shares) != 1 || l.Shares[0].Name != "Claude Code" {
		t.Errorf("shares %+v", l.Shares)
	}
	if _, err := ParseClaudeLimits([]byte(`{}`)); err == nil {
		t.Error("empty answer parsed")
	}
	for in, want := range map[[2]string]string{{"max", "default_claude_max_5x"}: "MAX 5X", {"max", "default_claude_max_20x"}: "MAX 20X", {"pro", ""}: "PRO"} {
		if got := PlanName(in[0], in[1]); got != want {
			t.Errorf("PlanName(%v) = %q", in, got)
		}
	}
}
