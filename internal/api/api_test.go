package api

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/justynroberts/marquee/internal/engine"
)

func newServer(t *testing.T) (*Server, *engine.Engine) {
	t.Helper()
	dir := t.TempDir()
	paths, err := engine.NewPaths(dir)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	e, err := engine.New(engine.Options{Paths: paths, NoPanel: true, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	go e.Run()
	t.Cleanup(e.Stop)
	keys, err := LoadKeys(filepath.Join(dir, "keys.json"))
	if err != nil {
		t.Fatal(err)
	}
	return &Server{E: e, Keys: keys, Log: log, MessagesFile: paths.Messages}, e
}

// do sends a request as if from remote (LAN) or loopback.
func do(t *testing.T, h http.Handler, method, path, body, key string, remote bool) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.RemoteAddr = "127.0.0.1:5000"
	req.Host = "127.0.0.1:7781"
	if remote {
		req.RemoteAddr = "192.168.1.20:5000"
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestAuth(t *testing.T) {
	s, _ := newServer(t)
	h := s.Handler()
	if code, _ := do(t, h, "GET", "/v1/state", "", "", false); code != 200 {
		t.Fatalf("loopback without key: %d", code)
	}
	if code, _ := do(t, h, "GET", "/v1/state", "", "", true); code != 401 {
		t.Fatalf("remote without key: %d", code)
	}
	if code, _ := do(t, h, "POST", "/v1/keys", `{"name":"x"}`, "", true); code != 403 {
		t.Fatalf("remote key creation: %d", code)
	}
	code, out := do(t, h, "POST", "/v1/keys", `{"name":"laptop"}`, "", false)
	if code != 201 {
		t.Fatalf("create key: %d %v", code, out)
	}
	secret := out["secret"].(string)
	if code, _ := do(t, h, "GET", "/v1/state", "", secret, true); code != 200 {
		t.Fatalf("remote with key: %d", code)
	}
	if code, _ := do(t, h, "GET", "/v1/state", "", secret+"x", true); code != 401 {
		t.Fatalf("remote with bad key: %d", code)
	}
	id := out["key"].(map[string]any)["id"].(string)
	if code, _ := do(t, h, "DELETE", "/v1/keys/"+id, "", "", false); code != 200 {
		t.Fatalf("revoke: %d", code)
	}
	if code, _ := do(t, h, "GET", "/v1/state", "", secret, true); code != 401 {
		t.Fatalf("revoked key still works: %d", code)
	}
	s.RequireKey = true
	if code, _ := do(t, s.Handler(), "GET", "/v1/state", "", "", false); code != 401 {
		t.Fatalf("require-key loopback: %d", code)
	}
	if code, _ := do(t, s.Handler(), "GET", "/v1/openapi.json", "", "", true); code != 200 {
		t.Fatalf("openapi should be public: %d", code)
	}
}

func TestPlayFlashTimer(t *testing.T) {
	s, e := newServer(t)
	h := s.Handler()
	if code, out := do(t, h, "POST", "/v1/play", `{"program":"board"}`, "", false); code != 200 {
		t.Fatalf("play board: %d %v", code, out)
	}
	if e.Status().Program != "board" {
		t.Fatalf("program = %s", e.Status().Program)
	}
	if code, _ := do(t, h, "POST", "/v1/play", `{"program":"nope"}`, "", false); code != 404 {
		t.Fatalf("unknown program: %d", code)
	}
	if code, _ := do(t, h, "POST", "/v1/play", `not json`, "", false); code != 400 {
		t.Fatalf("bad json: %d", code)
	}
	if code, _ := do(t, h, "POST", "/v1/flash", `{"text":""}`, "", false); code != 400 {
		t.Fatalf("empty flash: %d", code)
	}
	if code, _ := do(t, h, "POST", "/api/flash", `{"text":"Hook compat","seconds":2,"colour":"green"}`, "", false); code != 200 {
		t.Fatalf("compat flash: %d", code)
	}
	if e.Status().Overlay != "flash" {
		t.Fatalf("overlay = %q", e.Status().Overlay)
	}
	if code, _ := do(t, h, "DELETE", "/v1/overlay", "", "", false); code != 200 {
		t.Fatal("clear overlay")
	}
	if code, _ := do(t, h, "POST", "/v1/timer", `{"minutes":0}`, "", false); code != 400 {
		t.Fatalf("zero timer: %d", code)
	}
	if code, _ := do(t, h, "POST", "/v1/timer", `{"seconds":30,"label":"tea"}`, "", false); code != 200 {
		t.Fatalf("timer: %d", code)
	}
	if code, out := do(t, h, "POST", "/v1/brightness", `{"percent":500}`, "", false); code != 200 || out["percent"].(float64) != 100 {
		t.Fatalf("brightness clamp: %v", out)
	}
	code, out := do(t, h, "GET", "/v1/programs", "", "", false)
	if code != 200 || len(out["programs"].([]any)) < 5 {
		t.Fatalf("programs: %d %v", code, out)
	}
}

func TestMessagesAndSchedule(t *testing.T) {
	s, _ := newServer(t)
	h := s.Handler()
	if code, _ := do(t, h, "PUT", "/v1/messages", `{"text":"[red]hello\n[bogus]x"}`, "", false); code != 200 {
		t.Fatal("put messages")
	}
	if code, _ := do(t, h, "POST", "/v1/messages", `{"text":"line two\nwith newline"}`, "", false); code != 200 {
		t.Fatal("append message")
	}
	_, out := do(t, h, "GET", "/v1/messages", "", "", false)
	text := out["text"].(string)
	if !strings.Contains(text, "[red]hello") || !strings.Contains(text, "line two with newline") {
		t.Fatalf("messages = %q", text)
	}
	code, out := do(t, h, "PUT", "/v1/schedule", `{"text":"08:30 dashboard\n25:00 bad\n# 09:00 x\n12:00 flash Lunch"}`, "", false)
	if code != 200 || len(out["entries"].([]any)) != 2 {
		t.Fatalf("schedule: %d %v", code, out)
	}
}

func TestFrameAndMedia(t *testing.T) {
	s, e := newServer(t)
	h := s.Handler()
	img := image.NewRGBA(image.Rect(0, 0, 64, 16))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)

	req := httptest.NewRequest("POST", "/v1/frame", bytes.NewReader(buf.Bytes()))
	req.RemoteAddr = "127.0.0.1:1"
	req.Host = "127.0.0.1:7781"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || e.Status().Program != "push" {
		t.Fatalf("frame: %d %s", rec.Code, rec.Body)
	}
	req = httptest.NewRequest("POST", "/v1/frame", strings.NewReader("not an image"))
	req.RemoteAddr = "127.0.0.1:1"
	req.Host = "127.0.0.1:7781"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 415 {
		t.Fatalf("bad frame: %d", rec.Code)
	}
	req = httptest.NewRequest("POST", "/v1/media?name=../../evil.png", bytes.NewReader(buf.Bytes()))
	req.RemoteAddr = "127.0.0.1:1"
	req.Host = "127.0.0.1:7781"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("media upload: %d %s", rec.Code, rec.Body)
	}
	if e.Status().Program != "media:evil.png" {
		t.Fatalf("media program = %s", e.Status().Program)
	}
	time.Sleep(300 * time.Millisecond)
	if len(e.Preview()) < 100 {
		t.Fatal("no preview rendered")
	}
}

func TestPythonCompat(t *testing.T) {
	s, e := newServer(t)
	h := s.Handler()
	code, out := do(t, h, "GET", "/api/state", "", "", false)
	if code != 200 || out["programs"] == nil || out["media"] == nil || out["pagerduty"] == nil || out["flash"] != false {
		t.Fatalf("compat state: %d %v", code, out)
	}
	if code, _ := do(t, h, "POST", "/api/timer", `{}`, "", false); code != 200 || e.Status().Overlay != "timer" {
		t.Fatalf("timer default: %d %q", code, e.Status().Overlay)
	}
	if code, _ := do(t, h, "POST", "/api/pause", `{"paused":true}`, "", false); code != 200 || !e.Status().Paused {
		t.Fatal("compat pause")
	}
	for _, path := range []string{"/api/messages", "/api/schedule"} {
		if code, _ := do(t, h, "GET", path, "", "", false); code != 200 {
			t.Fatalf("GET %s: %d", path, code)
		}
	}
	// The Python server's token header and query parameter are accepted as API keys.
	_, secret, _ := s.Keys.Create("old remote")
	req := httptest.NewRequest("GET", "/api/state", nil)
	req.RemoteAddr = "10.0.0.5:1"
	req.Header.Set("X-Panel-Token", secret)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("X-Panel-Token: %d", rec.Code)
	}
	req = httptest.NewRequest("GET", "/api/preview.jpg?token="+secret, nil)
	req.RemoteAddr = "10.0.0.5:1"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("?token=: %d", rec.Code)
	}
	// Flash seconds clamp to at least 1, like the Python server.
	if code, _ := do(t, h, "POST", "/api/flash", `{"text":"x","seconds":0}`, "", false); code != 200 {
		t.Fatal("flash zero seconds")
	}
}
