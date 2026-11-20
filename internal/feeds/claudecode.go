package feeds

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Claude Code keeps every conversation as JSONL under ~/.claude/projects, and each assistant
// message there carries the API's token usage. ClaudeScanner reads those files on this Mac
// (nothing is sent anywhere): the first scan reads the last week, later scans read only what
// was appended since. Live sessions come from ~/.claude/sessions.

// ClaudeTokens is a token count by kind.
type ClaudeTokens struct {
	In, Out, CacheWrite, CacheRead int64
	Calls                          int
}

func (t *ClaudeTokens) add(u ccUsage, sign int64) {
	t.In += sign * u.In
	t.Out += sign * u.Out
	t.CacheWrite += sign * u.CW
	t.CacheRead += sign * u.CR
}

// CacheHit is the share of input read from the prompt cache.
func (t ClaudeTokens) CacheHit() float64 {
	all := t.In + t.CacheWrite + t.CacheRead
	if all == 0 {
		return 0
	}
	return float64(t.CacheRead) / float64(all)
}

// Named is a count with a name (a model or a project).
type Named struct {
	Name string
	ClaudeTokens
}

// ClaudeDay is one day's total.
type ClaudeDay struct {
	Day time.Time
	ClaudeTokens
}

// ClaudeSession is a running Claude Code session.
type ClaudeSession struct {
	ID, Name, Project, Model string
	Busy                     bool
	Since                    time.Time // status since
	Context                  int64     // tokens in the conversation at its last turn
	Today                    ClaudeTokens
}

// ClaudeUsage is today's use, the last seven days and what is running now.
type ClaudeUsage struct {
	Today    ClaudeTokens
	Hours    [24]ClaudeTokens
	Days     []ClaudeDay // seven, oldest first, today last
	Models   []Named     // today, most output first
	Projects []Named     // today, most output first
	Sessions []ClaudeSession
	// Week is output by hour for the seven days in Days; ProjectHours is today's output by
	// hour for each project.
	Week         [7][24]int64
	ProjectHours map[string][24]int64
	Recent       []ClaudeCall // the latest replies, oldest first
}

// ClaudeCall is one reply.
type ClaudeCall struct {
	At      time.Time
	Out     int64
	Project string
}

type ccUsage struct {
	In  int64 `json:"input_tokens"`
	Out int64 `json:"output_tokens"`
	CW  int64 `json:"cache_creation_input_tokens"`
	CR  int64 `json:"cache_read_input_tokens"`
}

type ccLine struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	SessionID string    `json:"sessionId"`
	Cwd       string    `json:"cwd"`
	Sidechain bool      `json:"isSidechain"`
	RequestID string    `json:"requestId"`
	Message   struct {
		ID    string   `json:"id"`
		Model string   `json:"model"`
		Usage *ccUsage `json:"usage"`
	} `json:"message"`
}

type ccSeen struct {
	at                   time.Time
	day                  string
	hour                 int
	model, project, sess string
	u                    ccUsage
}

type ccDay struct {
	tok      ClaudeTokens
	hours    [24]ClaudeTokens
	models   map[string]*ClaudeTokens
	projects map[string]*ClaudeTokens
	sessions map[string]*ClaudeTokens
	projHrs  map[string]*[24]int64
}

type ccContext struct {
	tokens int64
	model  string
	at     time.Time
}

// ClaudeScanner accumulates usage from the transcripts. Safe for one caller at a time.
type ClaudeScanner struct {
	Root string // ~/.claude

	mu      sync.Mutex
	offsets map[string]int64
	seen    map[string]*ccSeen
	days    map[string]*ccDay
	ctx     map[string]ccContext // by session id
	recent  []*ccSeen
}

func NewClaudeScanner(root string) *ClaudeScanner {
	return &ClaudeScanner{Root: root, offsets: map[string]int64{}, seen: map[string]*ccSeen{},
		days: map[string]*ccDay{}, ctx: map[string]ccContext{}}
}

// ClaudeHome is ~/.claude.
func ClaudeHome() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".claude")
}

const ccWindow = 8 * 24 * time.Hour

// Scan reads anything new and returns the usage as of now.
func (s *ClaudeScanner) Scan(now time.Time) (*ClaudeUsage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root := filepath.Join(s.Root, "projects")
	if _, err := os.Stat(root); err != nil {
		return nil, errors.New("no Claude Code history on this Mac")
	}
	cutoff := now.Add(-ccWindow)
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.ModTime().Before(cutoff) {
			return nil
		}
		s.readFile(path, info.Size(), cutoff)
		return nil
	})
	s.prune(now)
	return s.summary(now), nil
}

func (s *ClaudeScanner) readFile(path string, size int64, cutoff time.Time) {
	off := s.offsets[path]
	if size < off {
		off = 0 // rewritten
	}
	if size == off {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return
	}
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			break // an unfinished last line is read next time
		}
		off += int64(len(line))
		if bytes.Contains(line, []byte(`"usage"`)) && bytes.Contains(line, []byte(`"assistant"`)) {
			s.record(line, cutoff)
		}
	}
	s.offsets[path] = off
}

func (s *ClaudeScanner) record(line []byte, cutoff time.Time) {
	var l ccLine
	if json.Unmarshal(line, &l) != nil || l.Type != "assistant" || l.Message.Usage == nil {
		return
	}
	if l.Message.Model == "" || strings.HasPrefix(l.Message.Model, "<") || l.Timestamp.Before(cutoff) {
		return
	}
	u := *l.Message.Usage
	if !l.Sidechain && l.SessionID != "" {
		if c := s.ctx[l.SessionID]; l.Timestamp.After(c.at) {
			s.ctx[l.SessionID] = ccContext{u.In + u.CW + u.CR, l.Message.Model, l.Timestamp}
		}
	}
	// one message is written once per content block, with the same usage (output can grow)
	key := l.Message.ID + "|" + l.RequestID
	if old, ok := s.seen[key]; ok {
		if u.Out <= old.u.Out && u.In+u.CW+u.CR <= old.u.In+old.u.CW+old.u.CR {
			return
		}
		s.apply(old, -1)
		old.u = u
		s.apply(old, 1)
		return
	}
	lt := l.Timestamp.Local()
	e := &ccSeen{at: l.Timestamp, day: lt.Format("2006-01-02"), hour: lt.Hour(), model: l.Message.Model,
		project: filepath.Base(l.Cwd), sess: l.SessionID, u: u}
	if l.Cwd == "" {
		e.project = "?"
	}
	s.seen[key] = e
	s.apply(e, 1)
	s.recent = append(s.recent, e) // sorted and trimmed in summary
}

func (s *ClaudeScanner) apply(e *ccSeen, sign int64) {
	d := s.days[e.day]
	if d == nil {
		d = &ccDay{models: map[string]*ClaudeTokens{}, projects: map[string]*ClaudeTokens{}, sessions: map[string]*ClaudeTokens{}, projHrs: map[string]*[24]int64{}}
		s.days[e.day] = d
	}
	bump := func(m map[string]*ClaudeTokens, k string) {
		t := m[k]
		if t == nil {
			t = &ClaudeTokens{}
			m[k] = t
		}
		t.add(e.u, sign)
		t.Calls += int(sign)
	}
	d.tok.add(e.u, sign)
	d.tok.Calls += int(sign)
	d.hours[e.hour].add(e.u, sign)
	d.hours[e.hour].Calls += int(sign)
	bump(d.models, e.model)
	bump(d.projects, e.project)
	bump(d.sessions, e.sess)
	ph := d.projHrs[e.project]
	if ph == nil {
		ph = &[24]int64{}
		d.projHrs[e.project] = ph
	}
	ph[e.hour] += sign * e.u.Out
}

func (s *ClaudeScanner) prune(now time.Time) {
	keep := now.Add(-ccWindow).Format("2006-01-02")
	for k, e := range s.seen {
		if e.day < keep {
			delete(s.seen, k)
		}
	}
	for k := range s.days {
		if k < keep {
			delete(s.days, k)
		}
	}
}

func named(m map[string]*ClaudeTokens) []Named {
	var out []Named
	for k, t := range m {
		if t.Calls > 0 {
			out = append(out, Named{k, *t})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Out != out[j].Out {
			return out[i].Out > out[j].Out
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (s *ClaudeScanner) summary(now time.Time) *ClaudeUsage {
	u := &ClaudeUsage{}
	today := now.Format("2006-01-02")
	if d := s.days[today]; d != nil {
		u.Today, u.Hours = d.tok, d.hours
		u.Models, u.Projects = named(d.models), named(d.projects)
		u.ProjectHours = map[string][24]int64{}
		for k, v := range d.projHrs {
			u.ProjectHours[k] = *v
		}
	}
	mid := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for i := 6; i >= 0; i-- {
		day := mid.AddDate(0, 0, -i)
		cd := ClaudeDay{Day: day}
		if d := s.days[day.Format("2006-01-02")]; d != nil {
			cd.ClaudeTokens = d.tok
			for h := range 24 {
				u.Week[6-i][h] = d.hours[h].Out
			}
		}
		u.Days = append(u.Days, cd)
	}
	sort.SliceStable(s.recent, func(i, j int) bool { return s.recent[i].at.Before(s.recent[j].at) })
	if len(s.recent) > 200 {
		s.recent = append(s.recent[:0], s.recent[len(s.recent)-200:]...)
	}
	for _, e := range s.recent {
		u.Recent = append(u.Recent, ClaudeCall{At: e.at, Out: e.u.Out, Project: e.project})
	}
	u.Sessions = s.live(today)
	return u
}

// live reads ~/.claude/sessions/*.json and keeps the ones whose process is still running.
func (s *ClaudeScanner) live(today string) []ClaudeSession {
	files, _ := filepath.Glob(filepath.Join(s.Root, "sessions", "*.json"))
	var out []ClaudeSession
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var j struct {
			PID       int    `json:"pid"`
			SessionID string `json:"sessionId"`
			Cwd       string `json:"cwd"`
			Name      string `json:"name"`
			Status    string `json:"status"`
			Since     int64  `json:"statusUpdatedAt"`
			Started   int64  `json:"startedAt"`
		}
		if json.Unmarshal(b, &j) != nil || j.PID <= 0 || !alive(j.PID) {
			continue
		}
		cs := ClaudeSession{ID: j.SessionID, Name: j.Name, Project: filepath.Base(j.Cwd), Busy: j.Status == "busy"}
		if cs.Name == "" {
			cs.Name = cs.Project
		}
		if j.Since > 0 {
			cs.Since = time.UnixMilli(j.Since)
		} else if j.Started > 0 {
			cs.Since = time.UnixMilli(j.Started)
		}
		if c, ok := s.ctx[j.SessionID]; ok {
			cs.Context, cs.Model = c.tokens, c.model
		}
		if d := s.days[today]; d != nil {
			if t := d.sessions[j.SessionID]; t != nil {
				cs.Today = *t
			}
		}
		out = append(out, cs)
	}
	sort.Slice(out, func(i, k int) bool {
		if out[i].Busy != out[k].Busy {
			return out[i].Busy
		}
		return out[i].Today.Out > out[k].Today.Out
	})
	return out
}

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// ------------------------------------------------------------------ live activity

// Claude Code's hooks report each tool call to Marquee as it happens (see
// scripts/claude-hook.sh). ClaudeActivity keeps the latest of them in memory.

// ClaudeEvent is one hook report: kind is "tool" (a tool started), "done" (it finished),
// "prompt" (you sent a message) or "stop" (Claude finished its turn).
type ClaudeEvent struct {
	At                     time.Time
	Session, Project, Kind string
	Tool, Detail           string
}

// ClaudeNow is what one session is doing now.
type ClaudeNow struct {
	Project, Tool, Detail string
	State                 string // "tool", "thinking" or "idle"
	Since                 time.Time
	Calls                 int // tool calls this turn
}

// ActivityLog holds recent hook reports.
type ActivityLog struct {
	mu     sync.Mutex
	events []ClaudeEvent
	now    map[string]*ClaudeNow
}

// Activity is the log the API writes and the Claude screens read.
var Activity = &ActivityLog{}

// Add records a report.
func (a *ActivityLog) Add(ev ClaudeEvent) {
	if ev.At.IsZero() {
		ev.At = time.Now()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.now == nil {
		a.now = map[string]*ClaudeNow{}
	}
	n := a.now[ev.Session]
	if n == nil {
		n = &ClaudeNow{}
		a.now[ev.Session] = n
	}
	if ev.Project != "" {
		n.Project = ev.Project
	}
	switch ev.Kind {
	case "tool":
		n.State, n.Tool, n.Detail, n.Since = "tool", ev.Tool, ev.Detail, ev.At
		n.Calls++
	case "done":
		n.State, n.Since = "thinking", ev.At
	case "prompt":
		n.State, n.Tool, n.Detail, n.Since, n.Calls = "thinking", "", "", ev.At, 0
	case "stop":
		n.State, n.Since = "idle", ev.At
	}
	if ev.Kind != "done" {
		a.events = append(a.events, ev)
		if len(a.events) > 400 {
			a.events = append(a.events[:0], a.events[len(a.events)-200:]...)
		}
	}
	for id, s := range a.now {
		if time.Since(s.Since) > 6*time.Hour {
			delete(a.now, id)
		}
	}
}

// Recent returns up to n reports, newest last.
func (a *ActivityLog) Recent(n int) []ClaudeEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.events) < n {
		n = len(a.events)
	}
	return append([]ClaudeEvent(nil), a.events[len(a.events)-n:]...)
}

// Now returns each session's state by session id.
func (a *ActivityLog) Now() map[string]ClaudeNow {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]ClaudeNow, len(a.now))
	for id, n := range a.now {
		out[id] = *n
	}
	return out
}
