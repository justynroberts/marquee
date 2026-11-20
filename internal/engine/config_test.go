package engine

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettings(t *testing.T) {
	dir := t.TempDir()
	p, err := NewPaths(dir)
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(Options{Paths: p, NoPanel: true, Log: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	// a good value is saved and summarised
	sum, err := e.SetSetting("countdown", "Trip = 2027-03-01\nBirthday = 03-14")
	if err != nil || sum != "2 dates" {
		t.Fatalf("countdown: %q %v", sum, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "countdown.txt")); !strings.Contains(string(b), "Trip") {
		t.Fatalf("countdown.txt = %q", b)
	}
	// a bad one is refused and nothing is written
	if _, err := e.SetSetting("worldclock", "Home = Not/AZone"); err == nil {
		t.Fatal("bad zone accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "world-clock.txt")); err == nil {
		t.Fatal("bad zone written")
	}
	if _, err := e.SetSetting("trains", "Long Eaton"); err == nil {
		t.Fatal("station name accepted as a code")
	}
	// empty goes back to the default
	if _, err := e.SetSetting("countdown", ""); err != nil {
		t.Fatal(err)
	}
	for _, s := range e.Settings() {
		if s.ID == "countdown" && (s.Set || s.Current == "") {
			t.Fatalf("after reset: %+v", s)
		}
		if s.Kind == "secret" && s.Value != "" {
			t.Fatal("a secret was returned")
		}
	}
	if _, err := e.SetSetting("nope", "x"); err != ErrUnknownSetting {
		t.Fatal(err)
	}
}
