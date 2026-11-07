package engine

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/justynroberts/marquee/internal/led"
)

// DefaultSchedule is written when schedule.txt is missing. Every example is commented out.
const DefaultSchedule = `# Daily schedule: HH:MM action. Applied at that minute every day.
# Actions: a program name (show, green, audio, board, dashboard, draw, matrix, fireworks, ...),
#          brightness N, pause, resume, flash Some text, timer MINUTES Label
# Remove the leading "# " to enable an example.
# 08:30 dashboard
# 09:00 brightness 100
# 09:55 timer 5 standup
# 12:30 flash Lunch!
# 17:30 fireworks
# 18:00 green
# 23:00 brightness 20
`

// ScheduleEntry is one parsed line.
type ScheduleEntry struct {
	At     string `json:"at"`
	Action string `json:"action"`
}

// ReadSchedule parses schedule.txt (creating it if missing).
func (e *Engine) ReadSchedule() ([]ScheduleEntry, string) {
	data, err := os.ReadFile(e.paths.Schedule)
	if err != nil {
		_ = os.WriteFile(e.paths.Schedule, []byte(DefaultSchedule), 0o644)
		data = []byte(DefaultSchedule)
	}
	return ParseSchedule(string(data)), string(data)
}

// ParseSchedule reads "HH:MM action" lines, skipping comments and malformed lines.
func ParseSchedule(text string) []ScheduleEntry {
	var out []ScheduleEntry
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		at, action, ok := strings.Cut(line, " ")
		if !ok || len(at) != 5 || at[2] != ':' {
			continue
		}
		h, err1 := strconv.Atoi(at[:2])
		m, err2 := strconv.Atoi(at[3:])
		if err1 != nil || err2 != nil || h > 23 || m > 59 || strings.TrimSpace(action) == "" {
			continue
		}
		out = append(out, ScheduleEntry{At: at, Action: strings.TrimSpace(action)})
	}
	return out
}

// WriteSchedule replaces schedule.txt.
func (e *Engine) WriteSchedule(text string) ([]ScheduleEntry, error) {
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if err := os.WriteFile(e.paths.Schedule, []byte(text), 0o644); err != nil {
		return nil, err
	}
	return ParseSchedule(text), nil
}

func (e *Engine) runSchedule() {
	minute := time.Now().Format("15:04")
	if minute == e.schedMinute {
		return
	}
	e.schedMinute = minute
	entries, _ := e.ReadSchedule()
	for _, s := range entries {
		if s.At != minute {
			continue
		}
		if err := e.Apply(s.Action); err != nil {
			e.setError(fmt.Sprintf("schedule %s %s: %v", s.At, s.Action, err))
		}
	}
}

// Apply runs a schedule-style action string.
func (e *Engine) Apply(action string) error {
	verb, rest, _ := strings.Cut(strings.TrimSpace(action), " ")
	switch strings.ToLower(verb) {
	case "brightness":
		n, err := strconv.Atoi(strings.TrimSpace(rest))
		if err != nil {
			return fmt.Errorf("brightness needs a number")
		}
		e.SetBrightness(n)
	case "pause":
		e.SetPaused(true)
	case "resume":
		e.SetPaused(false)
	case "flash":
		text := strings.TrimSpace(rest)
		if text == "" {
			text = "!"
		}
		e.Flash(text, 15, led.Alert)
	case "timer":
		mins, label, _ := strings.Cut(strings.TrimSpace(rest), " ")
		m, err := strconv.ParseFloat(mins, 64)
		if err != nil || m <= 0 {
			return fmt.Errorf("timer needs minutes")
		}
		e.Timer(m*60, label)
	default:
		return e.Play(strings.TrimSpace(action))
	}
	return nil
}
