package main

import (
	"io"
	"net/http"
	"testing"
)

func TestAppSettingsRebind(t *testing.T) {
	a, err := NewApp(AppOptions{DataDir: t.TempDir(), NoPanel: true, Addr: "127.0.0.1:7798", Log: newLogger(io.Discard)})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err := a.Listen(); err != nil {
		t.Fatal(err)
	}
	get := func() int {
		res, err := http.Get("http://127.0.0.1:7798/v1/state")
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if c := get(); c != 200 {
		t.Fatalf("before: %d", c)
	}
	if err := a.Update(func(s *Settings) { s.RequireKey = true }); err != nil {
		t.Fatal(err)
	}
	if c := get(); c != 401 {
		t.Fatalf("require key not applied: %d", c)
	}
	// Saved and reloaded.
	b, err := NewApp(AppOptions{DataDir: a.Paths.Dir, NoPanel: true, Log: newLogger(io.Discard)})
	if err != nil {
		t.Fatal(err)
	}
	defer b.E.Stop()
	if !b.Settings().RequireKey {
		t.Fatal("setting not persisted")
	}
}
