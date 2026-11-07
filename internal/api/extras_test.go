package api

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlaylistsAPI(t *testing.T) {
	s, e := newServer(t)
	h := s.Handler()
	if c, out := do(t, h, "PUT", "/v1/playlists/focus", `{"items":["plasma","board","clock"],"seconds":15}`, "", false); c != 200 {
		t.Fatalf("put: %d %v", c, out)
	}
	if c, out := do(t, h, "GET", "/v1/playlists", "", "", false); c != 200 || out["focus"] == nil || out["show"] == nil {
		t.Fatalf("list: %d %v", c, out)
	}
	if c, _ := do(t, h, "POST", "/v1/play", `{"program":"focus"}`, "", false); c != 200 {
		t.Fatalf("play custom playlist: %d", c)
	}
	if e.Status().Program != "focus" {
		t.Fatalf("program %q", e.Status().Program)
	}
	for _, bad := range []string{
		`/v1/playlists/show|{"items":["plasma"]}`,     // built-in
		`/v1/playlists/fire|{"items":["plasma"]}`,     // a scene name
		`/v1/playlists/x|{"items":["nope"]}`,          // unknown item
		`/v1/playlists/x|{"items":[]}`,                // empty
		`/v1/playlists/Bad%20Name|{"items":["fire"]}`, // name
		`/v1/playlists/y|{"items":["show"]}`,          // nested playlist
	} {
		p := strings.SplitN(bad, "|", 2)
		if c, _ := do(t, h, "PUT", p[0], p[1], "", false); c != 400 {
			t.Errorf("%s: want 400, got %d", p[0], c)
		}
	}
	if c, _ := do(t, h, "DELETE", "/v1/playlists/focus", "", "", false); c != 200 {
		t.Fatal("delete")
	}
	if c, _ := do(t, h, "DELETE", "/v1/playlists/show", "", "", false); c != 404 {
		t.Fatal("deleting a built-in should 404")
	}
}

func TestWebhooks(t *testing.T) {
	s, e := newServer(t)
	h := s.Handler()
	if c, out := do(t, h, "POST", "/v1/webhook", `{"message":"Deploy done","colour":"green","seconds":3}`, "", false); c != 200 {
		t.Fatalf("generic: %d %v", c, out)
	}
	if e.Status().Overlay == "" {
		t.Fatal("no overlay after generic webhook")
	}
	if c, _ := do(t, h, "POST", "/v1/webhook", `{"foo":1}`, "", false); c != 400 {
		t.Fatal("empty webhook should 400")
	}
	// remote without a key is refused
	if c, _ := do(t, h, "POST", "/v1/webhook", `{"text":"x"}`, "", true); c != 401 {
		t.Fatalf("remote webhook without key: %d", c)
	}

	gh := func(event, body, query string) map[string]any {
		req := httptest.NewRequest("POST", "/v1/webhook/github"+query, strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:1"
		req.Host = "127.0.0.1:7781"
		req.Header.Set("X-GitHub-Event", event)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", event, rec.Code, rec.Body)
		}
		return decode(rec.Body.String())
	}
	fail := `{"action":"completed","repository":{"name":"marquee"},"workflow_run":{"name":"CI","head_branch":"main","conclusion":"failure"}}`
	if out := gh("workflow_run", fail, ""); out["shown"] != true || !strings.Contains(out["text"].(string), "CI failed · marquee · main") {
		t.Fatalf("failure: %v", out)
	}
	pass := strings.Replace(fail, "failure", "success", 1)
	if out := gh("workflow_run", pass, ""); out["shown"] != false {
		t.Fatalf("passes are quiet by default: %v", out)
	}
	if out := gh("workflow_run", pass, "?passes=1"); out["shown"] != true {
		t.Fatalf("passes=1: %v", out)
	}
	if out := gh("release", `{"action":"published","repository":{"name":"marquee"},"release":{"tag_name":"v1.0.0"}}`, ""); out["shown"] != true {
		t.Fatalf("release: %v", out)
	}
	if out := gh("push", `{}`, ""); out["shown"] != false {
		t.Fatalf("push should be ignored: %v", out)
	}
}

func TestSettingsAPI(t *testing.T) {
	s, _ := newServer(t)
	s.PagerDutyTokenFile = filepath.Join(t.TempDir(), "pagerduty-token")
	h := s.Handler()
	if c, out := do(t, h, "PUT", "/v1/settings", `{"pagerdutyToken":"  u+abc  "}`, "", false); c != 200 {
		t.Fatalf("put: %d %v", c, out)
	}
	b, _ := os.ReadFile(s.PagerDutyTokenFile)
	if string(b) != "u+abc\n" {
		t.Fatalf("token file %q", b)
	}
	if st, _ := os.Stat(s.PagerDutyTokenFile); st.Mode().Perm() != 0o600 {
		t.Fatalf("token file mode %v", st.Mode())
	}
	if c, _ := do(t, h, "PUT", "/v1/settings", `{"pagerdutyToken":""}`, "", false); c != 200 {
		t.Fatal("clear")
	}
	if _, err := os.Stat(s.PagerDutyTokenFile); !os.IsNotExist(err) {
		t.Fatal("token not removed")
	}
	if c, _ := do(t, h, "PUT", "/v1/settings", `{"openAtLogin":true}`, "", false); c != 501 {
		t.Fatal("openAtLogin without the app should be 501")
	}
	if c, _ := do(t, h, "GET", "/v1/settings", "", "", true); c != 403 {
		t.Fatal("settings from the network must be refused")
	}
	// cross-site browser request on loopback
	req := httptest.NewRequest("PUT", "/v1/settings", strings.NewReader(`{"pagerdutyToken":"evil"}`))
	req.RemoteAddr = "127.0.0.1:1"
	req.Host = "127.0.0.1:7781"
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("cross-site settings change: %d", rec.Code)
	}
	req.Header.Set("Origin", "http://127.0.0.1:7781")
	req.Body = httptest.NewRequest("PUT", "/", strings.NewReader(`{}`)).Body
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("same-origin settings: %d %s", rec.Code, rec.Body)
	}
	_ = time.Second
}

func decode(s string) map[string]any {
	var m map[string]any
	_ = json.Unmarshal([]byte(s), &m)
	return m
}

func TestHUDAPI(t *testing.T) {
	s, e := newServer(t)
	h := s.Handler()
	if c, out := do(t, h, "POST", "/v1/play", `{"program":"plasma"}`, "", false); c != 200 {
		t.Fatal(out)
	}
	if c, out := do(t, h, "POST", "/v1/hud", `{"on":true,"text":"On air"}`, "", false); c != 200 || out["on"] != true || out["mirror"] != "horizontal" || out["text"] != "On air" {
		t.Fatalf("on: %d %v", c, out)
	}
	if e.Status().Program != "hud" {
		t.Fatal("hud not playing")
	}
	if c, out := do(t, h, "POST", "/v1/hud", `{"mirror":"vertical"}`, "", false); c != 200 || out["mirror"] != "vertical" {
		t.Fatalf("mirror: %v", out)
	}
	if c, _ := do(t, h, "POST", "/v1/mirror", `{"mirror":"sideways"}`, "", false); c != 400 {
		t.Fatal("bad mirror accepted")
	}
	if e.Status().Mirror != "" {
		t.Fatal("HUD mirror leaked into the global mirror")
	}
	if c, out := do(t, h, "POST", "/v1/hud", `{"on":false}`, "", false); c != 200 || out["on"] != false {
		t.Fatalf("off: %v", out)
	}
	if e.Status().Program != "plasma" {
		t.Fatalf("did not return to plasma: %s", e.Status().Program)
	}
}

func TestBrowserAndRebindingNeedKey(t *testing.T) {
	s, _ := newServer(t)
	h := s.Handler()
	send := func(host, origin string) int {
		req := httptest.NewRequest("POST", "/v1/flash", strings.NewReader(`{"text":"hi"}`))
		req.RemoteAddr = "127.0.0.1:1"
		req.Host = host
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if c := send("127.0.0.1:7781", ""); c != 200 {
		t.Fatalf("local tool: %d", c)
	}
	if c := send("localhost:7781", "http://localhost:7781"); c != 200 {
		t.Fatalf("own web remote: %d", c)
	}
	if c := send("127.0.0.1:7781", "https://evil.example"); c != 401 {
		t.Fatalf("cross-site page: %d", c)
	}
	if c := send("evil.example:7781", ""); c != 401 {
		t.Fatalf("DNS rebinding: %d", c)
	}
}
