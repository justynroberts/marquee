package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeMarquee records the requests it gets and answers the /v1 routes the MCP tools use.
type fakeMarquee struct {
	mu   sync.Mutex
	hits []string
	body map[string]string
}

var fakeJPEG = []byte{0xff, 0xd8, 0xff, 0xe0, 'f', 'a', 'k', 'e', 0xff, 0xd9}

func (f *fakeMarquee) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data, _ := io.ReadAll(r.Body)
	key := r.Method + " " + r.URL.Path
	f.mu.Lock()
	f.hits = append(f.hits, key)
	f.body[key] = string(data)
	f.mu.Unlock()
	js := func(v any) { w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(v) }
	switch key {
	case "GET /v1/state":
		js(map[string]any{"program": "board", "brightness": 80, "paused": false, "fps": 30, "panel": "connected"})
	case "GET /v1/programs":
		js(map[string]any{"programs": []map[string]string{
			{"name": "showcase", "group": "playlist", "description": "tour"},
			{"name": "board", "group": "panel", "description": "Scrolling message board"},
		}})
	case "POST /v1/play":
		var b struct{ Program string }
		_ = json.Unmarshal(data, &b)
		if b.Program != "board" && b.Program != "showcase" {
			w.WriteHeader(404)
			js(map[string]string{"error": "unknown program " + b.Program})
			return
		}
		js(map[string]any{"ok": true})
	case "POST /v1/flash", "POST /v1/timer", "DELETE /v1/overlay", "POST /v1/frame", "POST /v1/pause", "POST /v1/flip":
		js(map[string]any{"ok": true})
	case "POST /v1/messages", "PUT /v1/messages":
		js(map[string]any{"ok": true, "warnings": []string{}})
	case "GET /v1/messages":
		js(map[string]any{"text": "hello\n"})
	case "POST /v1/brightness":
		js(map[string]any{"ok": true, "percent": 50})
	case "GET /v1/preview.jpg":
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(fakeJPEG)
	default:
		w.WriteHeader(404)
		js(map[string]string{"error": "not found"})
	}
}

func (f *fakeMarquee) get(key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.body[key]
}

// mcpSession runs serveMCP over in-memory pipes.
type mcpSession struct {
	t    *testing.T
	in   *io.PipeWriter
	out  *bufio.Reader
	done chan error
}

func startMCP(t *testing.T) (*mcpSession, *fakeMarquee) {
	t.Helper()
	fake := &fakeMarquee{body: map[string]string{}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	t.Setenv("MARQUEE_URL", srv.URL)
	t.Setenv("MARQUEE_KEY", "")
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	s := &mcpSession{t: t, in: inW, out: bufio.NewReader(outR), done: make(chan error, 1)}
	go func() {
		s.done <- serveMCP(inR, outW, slog.New(slog.NewTextHandler(io.Discard, nil)))
		outW.Close()
	}()
	t.Cleanup(func() { inW.Close() })
	return s, fake
}

func (s *mcpSession) send(line string) {
	s.t.Helper()
	if _, err := io.WriteString(s.in, line+"\n"); err != nil {
		s.t.Fatal(err)
	}
}

func (s *mcpSession) recv() map[string]any {
	s.t.Helper()
	type res struct {
		line string
		err  error
	}
	ch := make(chan res, 1)
	go func() { l, err := s.out.ReadString('\n'); ch <- res{l, err} }()
	select {
	case r := <-ch:
		if r.err != nil {
			s.t.Fatalf("read: %v", r.err)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(r.line), &m); err != nil {
			s.t.Fatalf("bad JSON %q: %v", r.line, err)
		}
		return m
	case <-time.After(5 * time.Second):
		s.t.Fatal("timed out waiting for a response")
	}
	return nil
}

func (s *mcpSession) request(id int, method string, params any) map[string]any {
	s.t.Helper()
	msg := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	data, _ := json.Marshal(msg)
	s.send(string(data))
	resp := s.recv()
	if got, _ := resp["id"].(float64); int(got) != id {
		s.t.Fatalf("response id %v, want %d: %v", resp["id"], id, resp)
	}
	return resp
}

func (s *mcpSession) callTool(id int, name string, args any) map[string]any {
	s.t.Helper()
	resp := s.request(id, "tools/call", map[string]any{"name": name, "arguments": args})
	if resp["error"] != nil {
		s.t.Fatalf("%s: JSON-RPC error %v", name, resp["error"])
	}
	return resp["result"].(map[string]any)
}

func TestMCPInitializeAndList(t *testing.T) {
	s, _ := startMCP(t)
	resp := s.request(1, "initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"}})
	res := resp["result"].(map[string]any)
	if res["protocolVersion"] != "2025-03-26" {
		t.Errorf("protocolVersion = %v, want echoed 2025-03-26", res["protocolVersion"])
	}
	if res["serverInfo"].(map[string]any)["name"] != "marquee" {
		t.Errorf("serverInfo = %v", res["serverInfo"])
	}
	if _, ok := res["capabilities"].(map[string]any)["tools"]; !ok {
		t.Errorf("capabilities = %v", res["capabilities"])
	}
	// A notification gets no reply: the next line read must be the ping's response.
	s.send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	ping := s.request(2, "ping", nil)
	if r, ok := ping["result"].(map[string]any); !ok || len(r) != 0 {
		t.Errorf("ping result = %v, want {}", ping["result"])
	}

	resp = s.request(3, "initialize", map[string]any{"protocolVersion": "1999-01-01"})
	if v := resp["result"].(map[string]any)["protocolVersion"]; v != "2025-06-18" {
		t.Errorf("unknown protocol answered with %v", v)
	}

	resp = s.request(4, "tools/list", map[string]any{})
	tools := resp["result"].(map[string]any)["tools"].([]any)
	want := []string{"panel_status", "list_programs", "play", "flash", "say", "set_messages", "get_messages",
		"timer", "clear_alert", "brightness", "pause", "flip", "show_image", "preview", "list_fonts"}
	got := map[string]map[string]any{}
	for _, x := range tools {
		m := x.(map[string]any)
		got[m["name"].(string)] = m
	}
	for _, name := range want {
		tool, ok := got[name]
		if !ok {
			t.Errorf("tool %s missing", name)
			continue
		}
		if d, _ := tool["description"].(string); len(d) < 20 {
			t.Errorf("%s: description too short: %q", name, d)
		}
		sch, ok := tool["inputSchema"].(map[string]any)
		if !ok || sch["type"] != "object" {
			t.Errorf("%s: inputSchema must be an object schema: %v", name, tool["inputSchema"])
			continue
		}
		props, ok := sch["properties"].(map[string]any)
		if !ok {
			t.Errorf("%s: schema has no properties map", name)
			continue
		}
		for pn, p := range props {
			pm := p.(map[string]any)
			if pm["type"] == nil || pm["description"] == nil {
				t.Errorf("%s.%s: property needs type and description: %v", name, pn, pm)
			}
		}
		if req, ok := sch["required"].([]any); ok {
			for _, r := range req {
				if _, ok := props[r.(string)]; !ok {
					t.Errorf("%s: required %v is not a property", name, r)
				}
			}
		}
	}
	if !strings.Contains(got["say"]["description"].(string), "[rainbow]") {
		t.Error("say description should carry the board markup cheat sheet")
	}

	resp = s.request(5, "no/such/method", nil)
	if e, ok := resp["error"].(map[string]any); !ok || e["code"].(float64) != -32601 {
		t.Errorf("unknown method: %v", resp)
	}
}

func TestMCPToolCalls(t *testing.T) {
	s, fake := startMCP(t)
	s.request(1, "initialize", map[string]any{"protocolVersion": "2025-06-18"})
	s.send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)

	res := s.callTool(2, "play", map[string]any{"name": "showcase"})
	if res["isError"] == true {
		t.Fatalf("play: %v", res)
	}
	if b := fake.get("POST /v1/play"); !strings.Contains(b, `"program":"showcase"`) {
		t.Errorf("play body = %s", b)
	}

	res = s.callTool(3, "play", map[string]any{"name": "nope"})
	if res["isError"] != true || !strings.Contains(toolText(res), "unknown program") {
		t.Errorf("play unknown: %v", res)
	}

	res = s.callTool(4, "flash", map[string]any{"text": "Deploy done 🚀", "seconds": 5, "colour": "green"})
	if res["isError"] == true {
		t.Fatalf("flash: %v", res)
	}
	var fb map[string]any
	_ = json.Unmarshal([]byte(fake.get("POST /v1/flash")), &fb)
	if fb["text"] != "Deploy done 🚀" || fb["seconds"] != 5.0 || fb["colour"] != "green" {
		t.Errorf("flash body = %v", fb)
	}

	res = s.callTool(5, "preview", map[string]any{})
	content := res["content"].([]any)
	img := content[0].(map[string]any)
	if img["type"] != "image" || img["mimeType"] != "image/jpeg" {
		t.Fatalf("preview content = %v", img)
	}
	if raw, err := base64.StdEncoding.DecodeString(img["data"].(string)); err != nil || string(raw) != string(fakeJPEG) {
		t.Errorf("preview data does not round-trip: %v", err)
	}

	res = s.callTool(6, "panel_status", nil)
	if !strings.Contains(toolText(res), `"program": "board"`) {
		t.Errorf("panel_status = %v", toolText(res))
	}
	res = s.callTool(7, "list_programs", nil)
	if !strings.Contains(toolText(res), "## playlist") || !strings.Contains(toolText(res), "- showcase") {
		t.Errorf("list_programs = %v", toolText(res))
	}

	res = s.callTool(8, "no_such_tool", map[string]any{})
	if res["isError"] != true {
		t.Errorf("unknown tool should be isError: %v", res)
	}
	res = s.callTool(9, "flash", map[string]any{"text": ""})
	if res["isError"] != true {
		t.Errorf("empty flash should be isError: %v", res)
	}
	res = s.callTool(10, "show_image", map[string]any{})
	if res["isError"] != true {
		t.Errorf("show_image with no source should be isError: %v", res)
	}
	res = s.callTool(11, "flip", map[string]any{"flipped": true})
	if res["isError"] == true || !strings.Contains(fake.get("POST /v1/flip"), `"flip":true`) {
		t.Errorf("flip: %v body %s", res, fake.get("POST /v1/flip"))
	}

	// A notification for tools/call must not produce output either.
	s.send(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"clear_alert"}}`)
	if r := s.request(12, "ping", nil); r["result"] == nil {
		t.Errorf("ping after notification: %v", r)
	}

	s.in.Close()
	select {
	case err := <-s.done:
		if err != nil {
			t.Errorf("serveMCP returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("serveMCP did not stop at EOF")
	}
}

func TestMCPShowImageFromURL(t *testing.T) {
	s, fake := startMCP(t)
	png := []byte("\x89PNG\r\n\x1a\nrest")
	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(png) }))
	defer imgSrv.Close()
	res := s.callTool(1, "show_image", map[string]any{"url": imgSrv.URL + "/a.png"})
	if res["isError"] == true {
		t.Fatalf("show_image: %v", res)
	}
	if fake.get("POST /v1/frame") != string(png) {
		t.Errorf("frame body = %q", fake.get("POST /v1/frame"))
	}
}

func TestMCPParseError(t *testing.T) {
	s, _ := startMCP(t)
	s.send(`{not json`)
	resp := s.recv()
	if e, ok := resp["error"].(map[string]any); !ok || e["code"].(float64) != -32700 {
		t.Errorf("parse error: %v", resp)
	}
}

func toolText(res map[string]any) string {
	var b strings.Builder
	for _, c := range res["content"].([]any) {
		if m := c.(map[string]any); m["type"] == "text" {
			b.WriteString(m["text"].(string))
		}
	}
	return b.String()
}
