package engine

import (
	"log/slog"
	"testing"
)

func TestWidgets(t *testing.T) {
	p, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(Options{Paths: p, NoPanel: true, Log: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.SetWidget(Widget{ID: "Build", Value: "PASS", Status: "passed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SetWidget(Widget{Label: "Grok Bot", Text: "drafting", Status: "degraded"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Widget{{ID: "x"}, {ID: "no spaces", Value: "1"}, {ID: "y", Value: "1", Status: "purple"}} {
		if _, err := e.SetWidget(bad); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	ws := e.Widgets()
	if len(ws) != 2 || ws[0].ID != "build" || ws[0].Status != "ok" || ws[1].ID != "grok-bot" || ws[1].Status != "warn" {
		t.Fatalf("widgets = %+v", ws)
	}
	// a fresh engine reads them back from widgets.json
	e2, _ := New(Options{Paths: p, NoPanel: true, Log: slog.New(slog.DiscardHandler)})
	if len(e2.Widgets()) != 2 {
		t.Fatal("not persisted")
	}
	if err := e.DeleteWidget("build"); err != nil || len(e.Widgets()) != 1 {
		t.Fatal("delete", err)
	}
}

func TestRelayParsing(t *testing.T) {
	if u, err := ParseRelay("ntfy.sh/marquee-a7f3k9q2x"); err != nil || u != "https://ntfy.sh/marquee-a7f3k9q2x" {
		t.Fatal(u, err)
	}
	if u, err := ParseRelay("marquee-a7f3k9q2x"); err != nil || u != "https://ntfy.sh/marquee-a7f3k9q2x" {
		t.Fatal(u, err)
	}
	if _, err := ParseRelay("ntfy.sh/short"); err == nil {
		t.Fatal("short topic accepted")
	}
	if ws := relayWidgets("", `{"id":"grok","value":"DONE","status":"ok"}`); len(ws) != 1 || ws[0].ID != "grok" {
		t.Fatal(ws)
	}
	if ws := relayWidgets("", `[{"id":"a","value":"1"},{"id":"b","value":"2"}]`); len(ws) != 2 {
		t.Fatal(ws)
	}
	if ws := relayWidgets("Grok Bot", "Retro drafted"); len(ws) != 1 || ws[0].ID != "grok-bot" || ws[0].Text != "Retro drafted" {
		t.Fatal(ws)
	}
}
