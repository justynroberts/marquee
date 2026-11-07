package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/justynroberts/marquee/internal/update"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/justynroberts/marquee/internal/api"
	"github.com/justynroberts/marquee/internal/audio"
	"github.com/justynroberts/marquee/internal/engine"
	"github.com/justynroberts/marquee/internal/feeds"
	"github.com/justynroberts/marquee/internal/led"
	"github.com/justynroberts/marquee/web"
)

// Port is Marquee's fixed HTTP port (the Claude Code hook and docs assume it).
const Port = 7781

// Settings are the app-level choices made in the menu bar (saved in settings.json).
type Settings struct {
	LAN        bool `json:"lan"`
	RequireKey bool `json:"requireKey"`
}

// App owns the engine, the HTTP server and the Bonjour advertisement.
type App struct {
	Log   *slog.Logger
	Paths engine.Paths
	E     *engine.Engine
	Keys  *api.Keys
	PD    *feeds.Feed

	mu        sync.Mutex
	settings  Settings
	addr      string // explicit --addr overrides settings
	httpSrv   *http.Server
	stopBonj  func()
	listenErr error

	updMu    sync.Mutex
	upd      update.Result
	updOnce  sync.Once
	onUpdate func(update.Result) // the menu's hook
}

// AppOptions configure NewApp.
type AppOptions struct {
	DataDir      string
	NoPanel      bool
	SceneSeconds float64
	Addr         string
	LAN          *bool // nil = use saved setting
	RequireKey   *bool
	Log          *slog.Logger
}

// NewApp loads state and starts the engine (not the HTTP server).
func NewApp(o AppOptions) (*App, error) {
	paths, err := engine.NewPaths(o.DataDir)
	if err != nil {
		return nil, err
	}
	keys, err := api.LoadKeys(filepath.Join(paths.Dir, "keys.json"))
	if err != nil {
		return nil, fmt.Errorf("keys: %w", err)
	}
	e, err := engine.New(engine.Options{Paths: paths, NoPanel: o.NoPanel, SceneSeconds: o.SceneSeconds, Log: o.Log})
	if err != nil {
		return nil, err
	}
	a := &App{Log: o.Log, Paths: paths, E: e, Keys: keys, addr: o.Addr, stopBonj: func() {}}

	if data, err := os.ReadFile(a.settingsPath()); err == nil {
		_ = json.Unmarshal(data, &a.settings)
	}
	if o.LAN != nil {
		a.settings.LAN = *o.LAN
	}
	if o.RequireKey != nil {
		a.settings.RequireKey = *o.RequireKey
	}

	a.PD = feeds.NewFeed(filepath.Join(paths.Dir, "pagerduty-token"))
	pd := a.PD
	pd.RegisterTokens()
	pd.OnNew(func(title, urgency string) {
		prefix := ""
		if urgency == "high" {
			prefix = "P1 "
		}
		e.Flash(prefix+"INCIDENT: "+title, 30, led.Alert)
	})
	if pd.Enabled() {
		pd.Start()
	}
	go e.Run()
	return a, nil
}

func (a *App) settingsPath() string { return filepath.Join(a.Paths.Dir, "settings.json") }

// Settings returns the current settings.
func (a *App) Settings() Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings
}

// Update changes settings, saves them and restarts the listener if needed.
func (a *App) Update(f func(*Settings)) error {
	a.mu.Lock()
	old := a.settings
	f(&a.settings)
	s := a.settings
	a.mu.Unlock()
	data, _ := json.MarshalIndent(s, "", "  ")
	_ = os.WriteFile(a.settingsPath(), data, 0o600)
	if s != old {
		return a.Listen()
	}
	return nil
}

// ListenAddr is where the server listens for the current settings.
func (a *App) ListenAddr() string {
	if a.addr != "" {
		return a.addr
	}
	if a.Settings().LAN {
		return fmt.Sprintf(":%d", Port)
	}
	return fmt.Sprintf("127.0.0.1:%d", Port)
}

// Listen (re)starts the HTTP server with the current settings.
func (a *App) Listen() error {
	a.shutdownHTTP()
	s := a.Settings()
	addr := a.ListenAddr()
	srv := &api.Server{E: a.E, Keys: a.Keys, Log: a.Log, LAN: s.LAN, RequireKey: s.RequireKey, Remote: web.Remote(), MessagesFile: a.Paths.Messages,
		PagerDutyTokenFile: a.PD.TokenPath, Settings: apiSettings{a}, Update: a.CheckUpdate,
		PagerDuty: func() map[string]any {
			enabled, errText, tokens := a.PD.State()
			out := map[string]any{"enabled": enabled, "error": errText}
			for k, v := range tokens {
				out[k] = v
			}
			return out
		}}
	var ln net.Listener
	var err error
	for i := 0; i < 10; i++ { // the previous listener may take a moment to release the port
		if ln, err = net.Listen("tcp", addr); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		err = fmt.Errorf("listen %s: %w (is Marquee already running?)", addr, err)
		a.mu.Lock()
		a.listenErr = err
		a.mu.Unlock()
		return err
	}
	hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	stop := func() {}
	if s.LAN {
		port := ln.Addr().(*net.TCPAddr).Port
		if st, err := api.Advertise(port, version); err != nil {
			a.Log.Warn("bonjour advertise failed", "err", err)
		} else {
			stop = st
			a.Log.Info("advertising on the network", "service", "_marquee._tcp", "port", port)
		}
	}
	a.mu.Lock()
	a.httpSrv, a.stopBonj, a.listenErr = hs, stop, nil
	a.mu.Unlock()
	go func() {
		if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.Log.Error("http server", "err", err)
		}
	}()
	a.Log.Info("listening", "addr", addr, "lan", s.LAN, "requireKey", s.RequireKey)
	return nil
}

// ListenError reports why the server is not listening, if it is not.
func (a *App) ListenError() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.listenErr
}

func (a *App) shutdownHTTP() {
	a.mu.Lock()
	hs, stop := a.httpSrv, a.stopBonj
	a.httpSrv, a.stopBonj = nil, func() {}
	a.mu.Unlock()
	stop()
	if hs != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = hs.Shutdown(ctx)
	}
}

// Close stops everything and releases the panel and microphone.
func (a *App) Close() {
	a.shutdownHTTP()
	a.E.Stop()
	audio.StopShared()
	a.Log.Info("stopped")
}

// RemoteURL is the local web remote.
func (a *App) RemoteURL() string { return fmt.Sprintf("http://127.0.0.1:%d", Port) }

// LANURLs lists http://<ip>:7781 for each non-loopback IPv4 address.
func LANURLs() []string {
	var out []string
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, ad := range addrs {
			if ipn, ok := ad.(*net.IPNet); ok && ipn.IP.To4() != nil && !ipn.IP.IsLinkLocalUnicast() {
				out = append(out, fmt.Sprintf("http://%s:%d", ipn.IP, Port))
			}
		}
	}
	return out
}

// apiSettings exposes the settings the HTTP API may change (from this Mac only).
type apiSettings struct{ a *App }

func (s apiSettings) Get() map[string]any {
	return map[string]any{"openAtLogin": loginEnabled()}
}

func (s apiSettings) SetOpenAtLogin(on bool) error {
	if on == loginEnabled() {
		return nil
	}
	return setLogin(on)
}

func (s apiSettings) PagerDutyChanged() {
	if s.a.PD.Enabled() {
		s.a.PD.Start()
		go s.a.PD.Poll()
	}
}

// ---------------------------------------------------------------- updates

// CheckUpdate returns the last update check, checking now if force or none has run yet.
func (a *App) CheckUpdate(force bool) update.Result {
	a.updMu.Lock()
	have := a.upd.Checked != ""
	r := a.upd
	a.updMu.Unlock()
	if have && !force {
		return r
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r = update.Check(ctx, version)
	a.updMu.Lock()
	a.upd = r
	hook := a.onUpdate
	a.updMu.Unlock()
	if r.Available {
		a.Log.Info("update available", "current", version, "latest", r.Latest.Version)
	}
	if hook != nil {
		hook(r)
	}
	return r
}

// StartUpdateChecks checks a minute after launch and then daily.
func (a *App) StartUpdateChecks() {
	a.updOnce.Do(func() {
		go func() {
			time.Sleep(time.Minute)
			for {
				a.CheckUpdate(true)
				time.Sleep(24 * time.Hour)
			}
		}()
	})
}
