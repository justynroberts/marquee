package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/justynroberts/marquee/internal/engine"
	"github.com/justynroberts/marquee/internal/led"
	"github.com/justynroberts/marquee/internal/nowplaying"
)

// ---------------------------------------------------------------- playlists

func (s *Server) listPlaylists(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.E.AllPlaylists())
}

func (s *Server) putPlaylist(w http.ResponseWriter, r *http.Request) {
	var body engine.Playlist
	if err := readJSON(r, &body); err != nil {
		fail(w, 400, err.Error())
		return
	}
	name := strings.ToLower(r.PathValue("name"))
	if err := s.E.SetPlaylist(name, body); err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "name": name})
}

func (s *Server) deletePlaylist(w http.ResponseWriter, r *http.Request) {
	if err := s.E.DeletePlaylist(strings.ToLower(r.PathValue("name"))); err != nil {
		fail(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------------------------------------------------------------- news

func (s *Server) refreshNews(w http.ResponseWriter, r *http.Request) {
	if err := s.E.RefreshNews(); err != nil {
		fail(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------------------------------------------------------------- settings (this Mac only)

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"lan": s.LAN, "requireKey": s.RequireKey, "pagerduty": s.pagerduty()}
	if s.Settings != nil {
		for k, v := range s.Settings.Get() {
			out[k] = v
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OpenAtLogin    *bool   `json:"openAtLogin"`
		PagerDutyToken *string `json:"pagerdutyToken"`
	}
	if err := readJSON(r, &body); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if body.PagerDutyToken != nil {
		if s.PagerDutyTokenFile == "" {
			fail(w, 501, "PagerDuty is not available in this mode")
			return
		}
		tok := strings.TrimSpace(*body.PagerDutyToken)
		var err error
		if tok == "" {
			err = os.Remove(s.PagerDutyTokenFile)
			if errors.Is(err, os.ErrNotExist) {
				err = nil
			}
		} else {
			err = os.WriteFile(s.PagerDutyTokenFile, []byte(tok+"\n"), 0o600)
		}
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		if s.Settings != nil {
			s.Settings.PagerDutyChanged()
		}
	}
	if body.OpenAtLogin != nil {
		if s.Settings == nil {
			fail(w, 501, "open at login is only available in the Mac app")
			return
		}
		if err := s.Settings.SetOpenAtLogin(*body.OpenAtLogin); err != nil {
			fail(w, 500, err.Error())
			return
		}
	}
	s.getSettings(w, r)
}

// AppSettings is implemented by the menu-bar app for settings the API may change.
type AppSettings interface {
	Get() map[string]any
	SetOpenAtLogin(bool) error
	PagerDutyChanged()
}

// ---------------------------------------------------------------- webhooks

// webhook is the generic hook: {"text": "...", "colour": "green", "seconds": 8}, or any JSON
// with a "text", "message", "title" or "summary" field, or a plain-text body.
func (s *Server) webhook(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	text, colour, secs := "", "", 10.0
	var body map[string]any
	if json.Unmarshal(raw, &body) == nil {
		for _, k := range []string{"text", "message", "title", "summary", "content"} {
			if v, ok := body[k].(string); ok && strings.TrimSpace(v) != "" {
				text = v
				break
			}
		}
		colour, _ = body["colour"].(string)
		if c, ok := body["color"].(string); ok && colour == "" {
			colour = c
		}
		if v, ok := body["seconds"].(float64); ok {
			secs = v
		}
	} else if strings.HasPrefix(r.Header.Get("Content-Type"), "text/") {
		text = string(raw)
	}
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		fail(w, 400, "no text, message, title or summary in the body")
		return
	}
	c := led.Alert
	if colour != "" {
		pc, ok := led.ParseColour(colour)
		if !ok {
			fail(w, 400, fmt.Sprintf("unknown colour %q", colour))
			return
		}
		c = pc
	}
	s.E.Flash(clip(text, 300), max(1, min(secs, 600)), c)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

var (
	ghGreen = led.RGB{R: 70, G: 220, B: 110}
	ghAmber = led.RGB{R: 255, G: 190, B: 40}
)

// github turns GitHub webhook deliveries into flashes: failed workflow runs, releases,
// new issues and pull requests, merges, stars. Point a repository or organisation webhook (JSON) at
// /v1/webhook/github?token=KEY (add &passes=1 to also show passing runs).
func (s *Server) github(w http.ResponseWriter, r *http.Request) {
	event := r.Header.Get("X-GitHub-Event")
	var p ghPayload
	if err := json.NewDecoder(io.LimitReader(r.Body, 5<<20)).Decode(&p); err != nil {
		fail(w, 400, "request body must be JSON: "+err.Error())
		return
	}
	text, colour, secs := githubFlash(event, p)
	// Passing runs are noise unless asked for (?passes=1); failures always show.
	if colour == ghGreen && p.WorkflowRun.Conclusion == "success" && r.URL.Query().Get("passes") == "" {
		text = ""
	}
	if text == "" {
		writeJSON(w, 200, map[string]any{"ok": true, "shown": false})
		return
	}
	s.E.Flash(clip(text, 300), secs, colour)
	writeJSON(w, 200, map[string]any{"ok": true, "shown": true, "text": text})
}

type ghPayload struct {
	Action string `json:"action"`
	Repo   struct {
		Name string `json:"name"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
	WorkflowRun struct {
		Name       string `json:"name"`
		Branch     string `json:"head_branch"`
		Conclusion string `json:"conclusion"`
		Status     string `json:"status"`
		Event      string `json:"event"`
		Previous   *struct {
			Conclusion string `json:"conclusion"`
		} `json:"previous_attempt"`
	} `json:"workflow_run"`
	Release struct {
		TagName string `json:"tag_name"`
		Name    string `json:"name"`
	} `json:"release"`
	Issue struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
	} `json:"issue"`
	PR struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Merged bool   `json:"merged"`
	} `json:"pull_request"`
	Zen string `json:"zen"`
}

func githubFlash(event string, p ghPayload) (string, led.RGB, float64) {
	repo := p.Repo.Name
	switch event {
	case "ping":
		return "GitHub connected · " + repo + " ✅", ghGreen, 6
	case "workflow_run":
		wr := p.WorkflowRun
		if p.Action != "completed" {
			return "", led.RGB{}, 0
		}
		switch wr.Conclusion {
		case "failure", "timed_out", "startup_failure":
			return fmt.Sprintf("🔥 %s failed · %s · %s", wr.Name, repo, wr.Branch), led.Alert, 30
		case "success":
			return fmt.Sprintf("✅ %s passed · %s · %s", wr.Name, repo, wr.Branch), ghGreen, 6
		}
	case "release":
		if p.Action == "published" {
			name := p.Release.TagName
			return fmt.Sprintf("🚀 %s %s released", repo, name), ghGreen, 15
		}
	case "issues":
		if p.Action == "opened" {
			return fmt.Sprintf("Issue #%d · %s · %s", p.Issue.Number, repo, p.Issue.Title), ghAmber, 12
		}
	case "pull_request":
		switch {
		case p.Action == "opened":
			return fmt.Sprintf("PR #%d · %s · %s", p.PR.Number, repo, p.PR.Title), ghAmber, 12
		case p.Action == "closed" && p.PR.Merged:
			return fmt.Sprintf("Merged #%d · %s · %s", p.PR.Number, repo, p.PR.Title), ghGreen, 10
		}
	case "star":
		if p.Action == "created" {
			return fmt.Sprintf("⭐ %s starred %s", p.Sender.Login, repo), ghAmber, 8
		}
	}
	return "", led.RGB{}, 0
}

// ---------------------------------------------------------------- HUD and mirror

func (s *Server) hud(w http.ResponseWriter, r *http.Request) {
	var body struct {
		On     *bool   `json:"on"`
		Text   *string `json:"text"`
		Mirror *string `json:"mirror"`
		Lyrics *bool   `json:"lyrics"`
	}
	if err := readJSON(r, &body); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if body.Text != nil {
		s.E.SetHUDText(*body.Text)
	}
	if body.Lyrics != nil {
		s.E.SetHUDLyrics(*body.Lyrics)
	}
	if body.On != nil {
		s.E.SetHUD(*body.On)
	}
	if body.Mirror != nil {
		if err := s.E.SetHUDMirror(*body.Mirror); err != nil {
			fail(w, 400, err.Error())
			return
		}
	}
	st := s.E.Status()
	writeJSON(w, 200, map[string]any{"ok": true, "on": s.E.HUDOn(), "text": st.HUDText, "mirror": s.E.HUDMirror(), "lyrics": st.HUDLyrics})
}

func (s *Server) mirror(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mirror string `json:"mirror"`
	}
	if err := readJSON(r, &body); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if err := s.E.SetMirror(body.Mirror); err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "mirror": s.E.Status().Mirror})
}

func (s *Server) lyrics(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Lead   *float64 `json:"lead"`
		Adjust *float64 `json:"adjust"`
	}
	if r.ContentLength != 0 {
		if err := readJSON(r, &body); err != nil && !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "EOF") {
			fail(w, 400, err.Error())
			return
		}
	}
	v := nowplaying.Lead()
	switch {
	case body.Lead != nil:
		v = s.E.SetLyricsLead(*body.Lead)
	case body.Adjust != nil:
		v = s.E.SetLyricsLead(v + *body.Adjust)
	}
	writeJSON(w, 200, map[string]any{"ok": true, "lead": v})
}

// updateCheck: GET /v1/update reports the last check; ?check=1 checks GitHub now.
func (s *Server) updateCheck(w http.ResponseWriter, r *http.Request) {
	if s.Update == nil {
		fail(w, 501, "update checks are only available in the app")
		return
	}
	writeJSON(w, 200, s.Update(r.URL.Query().Get("check") != ""))
}
