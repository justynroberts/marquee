package feeds

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClaudeScanner(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "projects", "-Users-me-work-app")
	os.MkdirAll(proj, 0o755)
	os.MkdirAll(filepath.Join(root, "sessions"), 0o755)
	now := time.Now()
	at := now.Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	line := func(id string, out int) string {
		return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"sessionId":"s1","cwd":"/Users/me/work/app","requestId":"r%s","message":{"id":%q,"model":"claude-opus-5-5","usage":{"input_tokens":10,"output_tokens":%d,"cache_creation_input_tokens":100,"cache_read_input_tokens":1000}}}`+"\n", at, id, id, out)
	}
	// the same message twice (two content blocks), its output growing, then another
	data := line("m1", 5) + line("m1", 50) + `{"type":"user","message":{"content":"hi"}}` + "\n" + line("m2", 7)
	path := filepath.Join(proj, "s1.jsonl")
	os.WriteFile(path, []byte(data), 0o644)
	os.WriteFile(filepath.Join(root, "sessions", "1.json"),
		[]byte(fmt.Sprintf(`{"pid":%d,"sessionId":"s1","cwd":"/Users/me/work/app","name":"app","status":"busy"}`, os.Getpid())), 0o644)

	s := NewClaudeScanner(root)
	u, err := s.Scan(now)
	if err != nil {
		t.Fatal(err)
	}
	if u.Today.Out != 57 || u.Today.Calls != 2 || u.Today.CacheRead != 2000 {
		t.Fatalf("today = %+v", u.Today)
	}
	if len(u.Projects) != 1 || u.Projects[0].Name != "app" {
		t.Fatalf("projects = %+v", u.Projects)
	}
	if len(u.Sessions) != 1 || !u.Sessions[0].Busy || u.Sessions[0].Context != 1110 || u.Sessions[0].Today.Out != 57 {
		t.Fatalf("sessions = %+v", u.Sessions)
	}
	// appended lines are read on the next scan, and only those
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(line("m3", 3))
	f.Close()
	if u, _ = s.Scan(now); u.Today.Out != 60 || u.Today.Calls != 3 {
		t.Fatalf("after append = %+v", u.Today)
	}
	if len(u.Days) != 7 || u.Days[6].Out != 60 {
		t.Fatalf("days = %+v", u.Days)
	}
}
