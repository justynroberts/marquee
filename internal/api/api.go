// Package api is Marquee's HTTP API (v1), the event stream, Bonjour advertising, and the web
// remote. Requests from this machine need no key; requests from the network need an API key.
package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	_ "golang.org/x/image/webp"

	"github.com/justynroberts/marquee/internal/board"
	"github.com/justynroberts/marquee/internal/engine"
	"github.com/justynroberts/marquee/internal/led"
	"github.com/justynroberts/marquee/internal/update"
)

// Version is set by the build.
var Version = "dev"

// Server wires the engine to HTTP.
type Server struct {
	E          *engine.Engine
	Keys       *Keys
	Log        *slog.Logger
	LAN        bool // listening beyond localhost
	RequireKey bool // require a key even on localhost
	Remote     http.Handler
	// PagerDuty reports the incident feed's state for /v1/state (optional).
	PagerDuty func() map[string]any
	// MessagesFile is the message board's messages.txt.
	MessagesFile string
	// PagerDutyTokenFile is where PUT /v1/settings stores the PagerDuty token (optional).
	PagerDutyTokenFile string
	// Settings lets the API change app settings such as open at login (optional).
	Settings AppSettings
	// Update reports (force: runs) the update check (optional).
	Update func(force bool) update.Result
}

const maxUpload = 25 << 20

// Handler builds the router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	v := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, s.auth(h)) }

	v("GET /v1", s.info)
	v("GET /v1/state", s.state)
	v("GET /v1/programs", s.programs)
	v("POST /v1/play", s.play)
	v("POST /v1/flash", s.flash)
	v("POST /v1/timer", s.timer)
	v("DELETE /v1/overlay", s.clearOverlay)
	v("GET /v1/messages", s.getMessages)
	v("PUT /v1/messages", s.putMessages)
	v("POST /v1/messages", s.addMessage)
	v("GET /v1/schedule", s.getSchedule)
	v("PUT /v1/schedule", s.putSchedule)
	v("GET /v1/media", s.listMedia)
	v("POST /v1/media", s.uploadMedia)
	v("DELETE /v1/media/{name}", s.deleteMedia)
	v("POST /v1/frame", s.frame)
	v("POST /v1/draw", s.draw)
	v("POST /v1/brightness", s.brightness)
	v("POST /v1/pause", s.pause)
	v("POST /v1/flip", s.flip)
	v("GET /v1/preview.jpg", s.preview)
	v("GET /v1/events", s.events)
	v("GET /v1/fonts", s.fonts)
	v("GET /v1/playlists", s.listPlaylists)
	v("PUT /v1/playlists/{name}", s.putPlaylist)
	v("DELETE /v1/playlists/{name}", s.deletePlaylist)
	v("POST /v1/news/refresh", s.refreshNews)
	v("POST /v1/hud", s.hud)
	v("POST /v1/mirror", s.mirror)
	v("POST /v1/lyrics", s.lyrics)
	v("GET /v1/update", s.updateCheck)
	v("POST /v1/webhook", s.webhook)
	v("POST /v1/webhook/github", s.github)
	mux.Handle("GET /v1/settings", s.localOnly(s.getSettings))
	mux.Handle("PUT /v1/settings", s.localOnly(s.putSettings))
	mux.HandleFunc("GET /v1/openapi.json", s.openapi) // public: describes the API
	mux.Handle("GET /v1/keys", s.localOnly(s.listKeys))
	mux.Handle("POST /v1/keys", s.localOnly(s.createKey))
	mux.Handle("DELETE /v1/keys/{id}", s.localOnly(s.revokeKey))
	// Compatibility with the earlier Python panel server (same paths and response shapes), so
	// existing scripts, the Claude Code hook and old remotes keep working.
	v("GET /api/state", s.compatState)
	v("GET /api/preview.jpg", s.preview)
	v("POST /api/play", s.play)
	v("POST /api/flash", s.flash)
	v("POST /api/say", s.addMessage)
	v("GET /api/messages", s.getMessages)
	v("PUT /api/messages", s.putMessages)
	v("POST /api/media", s.uploadMedia)
	v("POST /api/brightness", s.brightness)
	v("POST /api/timer", s.timer)
	v("DELETE /api/overlay", s.clearOverlay)
	v("POST /api/draw", s.draw)
	v("GET /api/schedule", s.getSchedule)
	v("PUT /api/schedule", s.putSchedule)
	v("POST /api/pause", s.pause)
	if s.Remote != nil {
		mux.Handle("GET /", s.Remote)
	}
	return s.logged(cors(mux))
}

// ---------------------------------------------------------------- middleware

// trustedLocal is a request from a program on this machine: a loopback peer, addressed to a
// loopback host (not a DNS-rebound name), and not sent by a web page on another origin. Only
// these get keyless access; browsers on other sites must present an API key like any device.
func trustedLocal(r *http.Request) bool {
	if !isLoopback(r) || !loopbackHost(r.Host) {
		return false
	}
	o := r.Header.Get("Origin")
	return o == "" || loopbackOrigin(o)
}

func loopbackHost(hostport string) bool {
	h := hostport
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		h = host
	}
	h = strings.Trim(h, "[]")
	if h == "" || strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func bearer(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	for _, h := range []string{"X-Marquee-Key", "X-Panel-Token"} {
		if k := r.Header.Get(h); k != "" {
			return k
		}
	}
	// For <img src> and EventSource, which cannot set headers ("token" is the Python server's name).
	if k := r.URL.Query().Get("key"); k != "" {
		return k
	}
	return r.URL.Query().Get("token")
}

func (s *Server) auth(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if trustedLocal(r) && !s.RequireKey {
			h(w, r)
			return
		}
		if s.Keys.Check(bearer(r)) {
			h(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="marquee"`)
		fail(w, http.StatusUnauthorized, "an API key is required from other devices: Authorization: Bearer mq_...")
	})
}

func (s *Server) localOnly(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopback(r) {
			fail(w, http.StatusForbidden, "keys and settings can only be changed from the machine running Marquee")
			return
		}
		// A web page in a local browser is on loopback too: refuse cross-site and DNS-rebound
		// requests.
		if !trustedLocal(r) {
			fail(w, http.StatusForbidden, "keys and settings cannot be changed from another web site")
			return
		}
		h(w, r)
	})
}

func loopbackOrigin(o string) bool {
	u, err := url.Parse(o)
	if err != nil {
		return false
	}
	h := u.Hostname()
	return h == "localhost" || net.ParseIP(h).IsLoopback()
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Marquee-Key, X-Panel-Token")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(c int) { w.code = c; w.ResponseWriter.WriteHeader(c) }
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) logged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: 200}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(sw, r)
		if strings.HasPrefix(r.URL.Path, "/v1/preview") || r.URL.Path == "/v1/state" || r.URL.Path == "/v1/events" {
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1") || strings.HasPrefix(r.URL.Path, "/api") {
			s.Log.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.code, "ms", time.Since(start).Milliseconds())
		}
	})
}

// ---------------------------------------------------------------- helpers

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("request body must be JSON: %v", err)
	}
	return nil
}

// ---------------------------------------------------------------- handlers

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	host, _ := os.Hostname()
	writeJSON(w, 200, map[string]any{
		"name": "Marquee", "version": Version, "host": host, "api": "v1",
		"panel": s.E.PanelInfo(), "docs": "/v1/openapi.json",
		"auth": map[string]any{"localhostNeedsKey": s.RequireKey, "network": s.LAN},
	})
}

func (s *Server) pagerduty() map[string]any {
	if s.PagerDuty == nil {
		return map[string]any{"enabled": false}
	}
	return s.PagerDuty()
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	st := s.E.Status()
	writeJSON(w, 200, struct {
		engine.Status
		PagerDuty map[string]any `json:"pagerduty"`
	}{st, s.pagerduty()})
}

// compatState is /api/state in the Python server's shape.
func (s *Server) compatState(w http.ResponseWriter, r *http.Request) {
	st := s.E.Status()
	var names []string
	for _, p := range s.E.Programs() {
		if p.Group != "media" {
			names = append(names, p.Name)
		}
	}
	var overlay any
	if st.Overlay != "" {
		overlay = st.Overlay
	}
	writeJSON(w, 200, map[string]any{
		"program": st.Program, "flash": st.Overlay != "", "overlay": overlay, "fps": st.FPS,
		"brightness": st.Brightness, "paused": st.Paused, "error": st.Error,
		"programs": nonNil(names), "media": nonNil(s.E.Media()), "pagerduty": s.pagerduty(),
	})
}

func (s *Server) programs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"programs": s.E.Programs()})
}

func (s *Server) play(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Program string `json:"program"`
	}
	if err := readJSON(r, &body); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if err := s.E.Play(body.Program); err != nil {
		code := 400
		if errors.Is(err, engine.ErrUnknownProgram) {
			code = 404
		}
		fail(w, code, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "program": body.Program})
}

func (s *Server) flash(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text    string   `json:"text"`
		Seconds *float64 `json:"seconds"`
		Colour  string   `json:"colour"`
		Color   string   `json:"color"`
	}
	if err := readJSON(r, &body); err != nil {
		fail(w, 400, err.Error())
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		fail(w, 400, "text is required")
		return
	}
	if len([]rune(text)) > 300 {
		text = string([]rune(text)[:300])
	}
	secs := 10.0
	if body.Seconds != nil {
		secs = max(1, min(*body.Seconds, 600))
	}
	colour := led.Alert
	if c, ok := led.ParseColour(body.Colour + body.Color); ok {
		colour = c
	}
	s.E.Flash(text, secs, colour)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) timer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Minutes *float64 `json:"minutes"`
		Seconds *float64 `json:"seconds"`
		Label   string   `json:"label"`
	}
	if err := readJSON(r, &body); err != nil {
		fail(w, 400, err.Error())
		return
	}
	total := 0.0
	if body.Minutes != nil {
		total += *body.Minutes * 60
	}
	if body.Seconds != nil {
		total += *body.Seconds
	}
	if body.Minutes == nil && body.Seconds == nil {
		total = 5 * 60 // the Python server's default
	}
	if total <= 0 || total > 86400 {
		fail(w, 400, "minutes (or seconds) must be between 0 and 24 hours")
		return
	}
	label := body.Label
	if len([]rune(label)) > 40 {
		label = string([]rune(label)[:40])
	}
	s.E.Timer(total, label)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) clearOverlay(w http.ResponseWriter, r *http.Request) {
	s.E.ClearOverlay()
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) messagesPath() string { return s.MessagesFile }

func (s *Server) getMessages(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.messagesPath())
	if err != nil {
		data = []byte(board.DefaultMessages)
	}
	writeJSON(w, 200, map[string]any{"text": string(data)})
}

func (s *Server) putMessages(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &body); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if len(body.Text) > 200_000 {
		fail(w, 413, "messages are limited to 200 KB")
		return
	}
	text := body.Text
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if err := os.WriteFile(s.messagesPath(), []byte(text), 0o644); err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "warnings": board.Lint(text)})
}

func (s *Server) addMessage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &body); err != nil {
		fail(w, 400, err.Error())
		return
	}
	text := strings.TrimSpace(strings.ReplaceAll(body.Text, "\n", " "))
	if text == "" {
		fail(w, 400, "text is required")
		return
	}
	if len([]rune(text)) > 500 {
		text = string([]rune(text)[:500])
	}
	if err := board.Append(s.messagesPath(), text); err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "warnings": board.Lint(text)})
}

func (s *Server) getSchedule(w http.ResponseWriter, r *http.Request) {
	entries, text := s.E.ReadSchedule()
	writeJSON(w, 200, map[string]any{"text": text, "entries": nonNil(entries)})
}

func (s *Server) putSchedule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &body); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if len(body.Text) > 50_000 {
		fail(w, 413, "schedule is limited to 50 KB")
		return
	}
	entries, err := s.E.WriteSchedule(body.Text)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "entries": nonNil(entries)})
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func (s *Server) listMedia(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"media": nonNil(s.E.Media())})
}

// uploadMedia accepts multipart (field "file") or a raw image body (Content-Type image/*, name from ?name=).
func (s *Server) uploadMedia(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	var name string
	var data []byte
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		f, hdr, err := r.FormFile("file")
		if err != nil {
			fail(w, 400, "expected a multipart field named file")
			return
		}
		defer f.Close()
		data, err = io.ReadAll(f)
		if err != nil {
			fail(w, 413, "files are limited to 25 MB")
			return
		}
		name = hdr.Filename
	} else {
		var err error
		data, err = io.ReadAll(r.Body)
		if err != nil {
			fail(w, 413, "files are limited to 25 MB")
			return
		}
		name = r.URL.Query().Get("name")
		if name == "" {
			name = "upload"
		}
	}
	prog, err := s.E.PlayMedia(name, data)
	if err != nil {
		fail(w, 415, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "program": prog})
}

func (s *Server) deleteMedia(w http.ResponseWriter, r *http.Request) {
	if err := s.E.DeleteMedia(r.PathValue("name")); err != nil {
		fail(w, 404, "no such media")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// frame shows one image pushed by an app (any size; cropped to fill). Send repeatedly to stream.
func (s *Server) frame(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	img, _, err := image.Decode(r.Body)
	if err != nil {
		fail(w, 415, "body must be a PNG, JPEG, GIF or WebP image")
		return
	}
	s.E.Push(img)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) draw(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PNG string `json:"png"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, 400, "body must be JSON {\"png\": \"data:image/png;base64,...\"}")
		return
	}
	data := body.PNG
	if i := strings.Index(data, ","); i >= 0 {
		data = data[i+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		fail(w, 400, "png must be base64")
		return
	}
	img, _, err := image.Decode(strings.NewReader(string(raw)))
	if err != nil {
		fail(w, 400, "png is not an image")
		return
	}
	s.E.Draw(img)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) brightness(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Percent *int `json:"percent"`
	}
	if err := readJSON(r, &body); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if body.Percent == nil {
		fail(w, 400, "percent is required (5-100)")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "percent": s.E.SetBrightness(*body.Percent)})
}

func (s *Server) pause(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Paused *bool `json:"paused"`
	}
	if err := readJSON(r, &body); err != nil {
		fail(w, 400, err.Error())
		return
	}
	p := true
	if body.Paused != nil {
		p = *body.Paused
	}
	s.E.SetPaused(p)
	writeJSON(w, 200, map[string]any{"ok": true, "paused": p})
}

func (s *Server) flip(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Flip bool `json:"flip"`
	}
	if err := readJSON(r, &body); err != nil {
		fail(w, 400, err.Error())
		return
	}
	s.E.SetFlip(body.Flip)
	writeJSON(w, 200, map[string]any{"ok": true, "flip": body.Flip})
}

func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(s.E.Preview())
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		fail(w, 500, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch, unsub := s.E.Subscribe()
	defer unsub()
	send := func(ev engine.Event) {
		data, _ := json.Marshal(ev.Data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
		fl.Flush()
	}
	send(engine.Event{Type: "state", Data: s.E.Status()})
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			send(ev)
		case <-tick.C:
			fmt.Fprint(w, ": keepalive\n\n")
			fl.Flush()
		}
	}
}

func (s *Server) fonts(w http.ResponseWriter, r *http.Request) {
	installed := make([]string, 0, len(led.InstalledFonts()))
	for _, p := range led.InstalledFonts() {
		b := p[strings.LastIndex(p, "/")+1:]
		installed = append(installed, strings.TrimSuffix(b, b[strings.LastIndex(b, "."):]))
	}
	writeJSON(w, 200, map[string]any{"shortcuts": led.FontNames(), "installed": installed})
}

func (s *Server) listKeys(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"keys": nonNil(s.Keys.List())})
}

func (s *Server) createKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	_ = readJSON(r, &body)
	if strings.TrimSpace(body.Name) == "" {
		body.Name = "unnamed"
	}
	key, secret, err := s.Keys.Create(body.Name)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	key.Hash = "" // never echo the stored hash
	writeJSON(w, 201, map[string]any{"key": key, "secret": secret, "note": "Store the secret now; it is not shown again."})
}

func (s *Server) revokeKey(w http.ResponseWriter, r *http.Request) {
	if !s.Keys.Revoke(r.PathValue("id")) {
		fail(w, 404, "no such key")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
