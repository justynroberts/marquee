// Package engine owns the panel: it runs the current program, overlays alerts and timers,
// applies brightness and the daily schedule, keeps a live preview, and publishes events.
package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/justynroberts/marquee/internal/dashboard"
	"github.com/justynroberts/marquee/internal/feeds"
	"github.com/justynroberts/marquee/internal/hud"
	"github.com/justynroberts/marquee/internal/led"
	"github.com/justynroberts/marquee/internal/location"
	"github.com/justynroberts/marquee/internal/nowplaying"
	"github.com/justynroberts/marquee/internal/panel"
	"github.com/justynroberts/marquee/internal/scenes"
)

// Paths are the files the engine reads and writes.
type Paths struct {
	Dir      string
	Messages string
	Schedule string
	State    string
	Media    string
}

// DefaultDataDir is ~/Library/Application Support/Marquee on macOS, $XDG_CONFIG_HOME/marquee elsewhere.
func DefaultDataDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		name := "marquee"
		if strings.Contains(d, "Library/Application Support") {
			name = "Marquee"
		}
		return filepath.Join(d, name)
	}
	return ".marquee"
}

// NewPaths creates the data directory layout.
func NewPaths(dir string) (Paths, error) {
	p := Paths{Dir: dir, Messages: filepath.Join(dir, "messages.txt"), Schedule: filepath.Join(dir, "schedule.txt"), State: filepath.Join(dir, "state.json"), Media: filepath.Join(dir, "media")}
	if err := os.MkdirAll(p.Media, 0o755); err != nil {
		return p, err
	}
	return p, nil
}

// Playlists are named sequences of scene names ("board" = one pass of the message board).
var Playlists = map[string][]string{
	"show":  {"matrix", "terminal", "board", "fireworks", "fire", "life", "warp", "board", "aquarium", "plasma", "rain", "clock", "bounce", "board"},
	"green": {"matrix", "terminal", "board"},
	"audio": {"mirror", "waterfall", "scope", "pulse", "meter", "spectrum"},
}

// State is what persists across restarts and what clients see.
type State struct {
	Program    string `json:"program"`
	Brightness int    `json:"brightness"`
	Paused     bool   `json:"paused"`
	Flip       bool   `json:"flip"`
	// Mirror flips everything sent to the panel, to read it reflected in a screen:
	// "horizontal", "vertical" or "".
	Mirror  string `json:"mirror,omitempty"`
	HUDText string `json:"hudText,omitempty"` // the HUD's message line
	HUDPrev string `json:"hudPrev,omitempty"` // program to return to when HUD mode ends
	// HUDMirror is how the HUD is flipped: "" (horizontal, the default), "horizontal",
	// "vertical" or "off". It applies whenever the HUD plays, however it was started.
	HUDMirror string `json:"hudMirror,omitempty"`
	// HUDLyrics shows synced lyrics on the HUD instead of the clock while a song has them.
	HUDLyrics bool `json:"hudLyrics,omitempty"`
	// LyricsLead is how many seconds early lyric lines appear (nil = the default).
	LyricsLead *float64 `json:"lyricsLead,omitempty"`
}

// Status is a snapshot for the API.
type Status struct {
	State
	Overlay   string  `json:"overlay,omitempty"`
	FPS       float64 `json:"fps"`
	Panel     string  `json:"panel"` // connected | disconnected | disabled
	PanelName string  `json:"panelName,omitempty"`
	Error     string  `json:"error,omitempty"`
	Timing    *Timing `json:"timing,omitempty"`
}

// Timing is how long the last couple of seconds of frames took, in milliseconds.
type Timing struct {
	EncodeMs  float64 `json:"encodeMs"`  // mean JPEG encode
	SendMs    float64 `json:"sendMs"`    // mean USB transfer, including the panel's acknowledgement
	SendMaxMs float64 `json:"sendMaxMs"` // slowest USB transfer
	FrameKB   float64 `json:"frameKB"`   // mean JPEG size
	Quality   int     `json:"quality"`   // current JPEG quality
}

// Event is published to subscribers (the API's event stream).
type Event struct {
	Type string `json:"type"` // state | overlay | panel | error
	Data any    `json:"data"`
}

// Options configure the engine.
type Options struct {
	Paths        Paths
	NoPanel      bool    // render and preview only
	SceneSeconds float64 // per scene in playlists
	Log          *slog.Logger
}

// Engine runs everything.
type Engine struct {
	opts  Options
	paths Paths
	log   *slog.Logger

	mu      sync.Mutex
	state   State
	program Program
	t0      time.Time
	overlay Program
	ovT0    time.Time
	draw    *drawProgram

	preview   []byte      // JPEG of lastFrame, encoded on demand
	lastFrame *image.RGBA // the frame most recently rendered
	frameSeq  uint64
	encSeq    uint64
	previewMu sync.Mutex
	fps       float64
	errText   string

	panel     *panel.Panel
	panelMode string
	nextTry   time.Time

	// Frames are encoded on the render goroutine and handed to a USB goroutine, so encoding
	// the next frame overlaps sending this one.
	usb     chan []byte
	quality int
	acc     timingAcc
	timing  *Timing

	subsMu sync.Mutex
	subs   map[chan Event]struct{}

	dashOnce sync.Once
	dash     *dashboard.Dashboard

	npOnce sync.Once
	np     *nowplaying.View

	custom   map[string]Playlist // playlists.json
	newsOnce sync.Once
	newsMu   sync.Mutex
	news     []feeds.Headline // the latest fetch, for the headlines wall
	newsAt   time.Time

	wmu     sync.Mutex
	widgets map[string]Widget // widgets.json, loaded on first use

	schedMinute string
	stop        chan struct{}
	done        chan struct{}
}

// New loads state and prepares the engine. Call Run to start it.
func New(o Options) (*Engine, error) {
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.SceneSeconds <= 0 {
		o.SceneSeconds = 20
	}
	e := &Engine{opts: o, paths: o.Paths, log: o.Log, subs: map[chan Event]struct{}{}, draw: &drawProgram{}, stop: make(chan struct{}), done: make(chan struct{})}
	e.state = State{Program: "show", Brightness: 100}
	if data, err := os.ReadFile(o.Paths.State); err == nil {
		_ = json.Unmarshal(data, &e.state)
	}
	if e.state.Brightness <= 0 {
		e.state.Brightness = 100
	}
	e.loadPlaylists()
	location.SetCacheFile(filepath.Join(e.paths.Dir, "location.json"))
	feeds.Home = e.macPlace
	if e.state.LyricsLead != nil {
		nowplaying.SetLead(*e.state.LyricsLead)
	}
	p, err := e.Make(e.state.Program)
	if err != nil {
		e.state.Program = "show"
		p, _ = e.Make("show")
	}
	e.program, e.t0 = p, time.Now()
	e.panelMode = "disconnected"
	if o.NoPanel {
		e.panelMode = "disabled"
	}
	return e, nil
}

// Programs lists everything Play accepts.
func (e *Engine) Programs() []ProgramInfo {
	var out []ProgramInfo
	names := make([]string, 0, len(Playlists))
	for n := range Playlists {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		out = append(out, ProgramInfo{Name: n, Group: "playlist", Description: strings.Join(Playlists[n], ", ")})
	}
	for _, n := range e.playlistNames() {
		if p, ok := e.customPlaylist(n); ok {
			out = append(out, ProgramInfo{Name: n, Group: "playlist", Description: strings.Join(p.Items, ", ")})
		}
	}
	out = append(out, ProgramInfo{Name: "showcase", Group: "playlist", Description: "A guided tour of everything Marquee can do"})
	out = append(out, ProgramInfo{Name: "board", Group: "panel", Description: "Scrolling message board (messages.txt)"},
		ProgramInfo{Name: "draw", Group: "panel", Description: "The drawing pad canvas"},
		ProgramInfo{Name: "news", Group: "news", Description: "Headline ticker: Hacker News, or the feeds in news-sources.txt"})
	out = append(out, ProgramInfo{Name: "hud", Group: "panel", Description: "Heads-up display: big clock and one line, high contrast (pair with mirror to read it reflected in a screen)"})
	out = append(out, ProgramInfo{Name: "nowplaying", Group: "scene", Description: "Now playing in Music or Spotify: artwork, title, progress (native resolution)"})
	out = append(out, ProgramInfo{Name: "karaoke", Group: "scene", Description: "Synced lyrics for what Music or Spotify is playing, each line lighting up as it is sung"})
	out = append(out, ProgramInfo{Name: "interference", Group: "scene", Description: "Live-stream interference field: drifting wave sources as contour bands (native resolution)"})
	out = append(out, ProgramInfo{Name: "bigclock", Group: "clocks", Description: "A full-resolution clock: hours and minutes as large as the panel allows, the date beside them"},
		ProgramInfo{Name: "calendar", Group: "scene", Description: "This month and next with week numbers and UK bank holidays (area in calendar.txt)"},
		ProgramInfo{Name: "wordclock", Group: "clocks", Description: "The time spelled out in a grid of letters"},
		ProgramInfo{Name: "flipclock", Group: "clocks", Description: "A split-flap board: each flap falls over its hinge as the digit turns"},
		ProgramInfo{Name: "nixieclock", Group: "clocks", Description: "Six nixie tubes: orange numerals glowing in front of the dark stack, neon dots blinking the seconds"},
		ProgramInfo{Name: "dialclock", Group: "clocks", Description: "The day as a radio tuning dial: the hours slide past a red needle, a fine scale of seconds beneath"},
		ProgramInfo{Name: "swarmclock", Group: "clocks", Description: "The time in thousands of particles that burst and stream to the new minute"},
		ProgramInfo{Name: "countdown", Group: "scene", Description: "Days to go until the dates in countdown.txt (Christmas and New Year by default)"},
		ProgramInfo{Name: "warroom", Group: "scene", Description: "A cold-war strategic simulation on a vector map: DEFCON rises, missiles fly, and every run ends with no winner"},
		ProgramInfo{Name: "claude", Group: "claude", Description: "Claude Code on this Mac: today's tokens, output by hour, the last seven days, and the sessions running now with how full each is"},
		ProgramInfo{Name: "claudelive", Group: "claude", Description: "What Claude Code is doing this moment, from its hooks: each session's current tool large, and the latest calls"},
		ProgramInfo{Name: "widgets", Group: "news", Description: "A board of readings pushed by scripts, CI or cloud tools: POST /v1/widgets, or through the relay"},
		ProgramInfo{Name: "claudehive", Group: "claude", Description: "Claude Code sessions as orbs: working ones spin and throw sparks into the Claude spark, idle ones doze, each ringed by how full it is"},
		ProgramInfo{Name: "claudecity", Group: "claude", Description: "Your week with Claude Code as a city at night: a tower for every hour, as tall as its output, this hour's under a crane"},
		ProgramInfo{Name: "claudeflow", Group: "claude", Description: "Today's Claude Code output as a river by project, sparks drifting downstream as fast as tokens arrive"},
		ProgramInfo{Name: "claudeclock", Group: "claude", Description: "The Claude spark as a 24-hour clock: one ray an hour, as long as that hour's output, the hour now growing as tokens arrive"},
		ProgramInfo{Name: "claudeorbit", Group: "claude", Description: "Claude Code sessions as planets orbiting the spark: size is today's output, the ring how full the conversation is"},
		ProgramInfo{Name: "claudeburst", Group: "claude", Description: "A firework for every reply Claude writes, as big as its output, in its project's colour"},
		ProgramInfo{Name: "claudediary", Group: "diary", Description: "Claude's diary: a quill writes how much of the plan is left, today's work, what it is doing and the week onto an old page, then the ink soaks away"},
		ProgramInfo{Name: "commonplace", Group: "diary", Description: "A commonplace book: quotations from writers long out of copyright, copied out by quill and signed in red, then soaked away"},
		ProgramInfo{Name: "naturediary", Group: "diary", Description: "A nature diary in quill and ink: the weather where you are, sunrise and sunset and how the days are drawing in or out, the moon, and the odd quotation"},
		ProgramInfo{Name: "newsdiary", Group: "diary", Description: "The day's papers written up by hand: BBC and Hacker News headlines and on this day in history"},
		ProgramInfo{Name: "diary", Group: "diary", Description: "A day book that writes itself: the weather, headlines, on this day, the moon and a little Claude, in quill and ink on an old page"},
		ProgramInfo{Name: "claudelimits", Group: "claude", Description: "How much of your Claude plan is left: the five-hour session and the week, with reset times, a pace mark and the week by app"},
		ProgramInfo{Name: "claudemeter", Group: "claude", Description: "Three big gauges: Claude's tokens a minute, the fullest conversation's context, and the cache hit rate"},
		ProgramInfo{Name: "progress", Group: "scene", Description: "Time as progress bars: this hour, day, week, month, quarter and year, ticking live"},
		ProgramInfo{Name: "pomodoro", Group: "scene", Description: "A focus (pomodoro) timer: work and break sessions with a countdown (lengths in focus.txt)"},
		ProgramInfo{Name: "daynight", Group: "scene", Description: "The world in day and night: the terminator and twilight on a map, with your place and the world-clock cities"},
		ProgramInfo{Name: "orrery", Group: "scene", Description: "The solar system now, seen from above: where each planet is, how far from Earth, and which are in the evening sky"},
		ProgramInfo{Name: "netwatch", Group: "scene", Description: "Internet health: latency and loss to your router, Cloudflare and Google DNS, and DNS lookup time"},
		ProgramInfo{Name: "sun", Group: "scene", Description: "Today's daylight where you are: the sun's path, dawn, sunrise, sunset, dusk and how the day is changing"},
		ProgramInfo{Name: "moon", Group: "scene", Description: "Tonight's moon: its phase drawn large, the next full and new moons, the coming 30 nights"},
		ProgramInfo{Name: "worldclock", Group: "clocks", Description: "Clocks for several cities with day/night strips (cities in world-clock.txt)"},
		ProgramInfo{Name: "headlines", Group: "news", Description: "A news wall: every headline in small type, four slowly scrolling columns (native resolution)"},
		ProgramInfo{Name: "flights", Group: "news", Description: "Aircraft overhead: a radar and the nearest planes, live from adsb.lol (place in weather.txt)"},
		ProgramInfo{Name: "quakes", Group: "news", Description: "Earthquakes worldwide in the last day and week on a map, from the USGS"},
		ProgramInfo{Name: "onthisday", Group: "news", Description: "On this day in history, from Wikipedia, one event at a time"},
		ProgramInfo{Name: "markets", Group: "news", Description: "Crypto prices from CoinGecko with the day's move and seven days (coins in markets.txt)"},
		ProgramInfo{Name: "grid", Group: "news", Description: "The electricity grid where you are: carbon per kWh, the generation mix, 48 hours ahead and the greenest time (GB)"},
		ProgramInfo{Name: "iss", Group: "news", Description: "Where the International Space Station is now: its ground track on a map, height, speed and distance from you"},
		ProgramInfo{Name: "launches", Group: "news", Description: "Rocket launches coming up worldwide, with a live countdown to the next, from Launch Library"},
		ProgramInfo{Name: "rain", Group: "news", Description: "Will it rain? Rain for the next two hours in 15-minute steps, and the next 12 hours (place in weather.txt)"},
		ProgramInfo{Name: "status", Group: "news", Description: "Are the services you rely on up? Status pages for GitHub, Cloudflare, Anthropic and more (list in status.txt)"},
		ProgramInfo{Name: "fx", Group: "news", Description: "Exchange rates with the day's move and 30 days, from the ECB via Frankfurter (currencies in fx.txt)"},
		ProgramInfo{Name: "aurora", Group: "news", Description: "Will you see the northern lights? AuroraWatch UK's alert level and NOAA's Kp forecast"},
		ProgramInfo{Name: "trains", Group: "news", Description: "Live departures and arrivals for a station from National Rail (station in trains.txt; needs a free Rail Data Marketplace key)"},
		ProgramInfo{Name: "air", Group: "news", Description: "Air quality and pollen for the weather place, now and the next 48 hours"},
		ProgramInfo{Name: "newsreel", Group: "news", Description: "One news source crawling across the panel in mid-size type (source in newsreel.txt)"},
		ProgramInfo{Name: "weather", Group: "scene", Description: "Weather now, the next 24 hours and the coming days (place in weather.txt)"},
		ProgramInfo{Name: "ascii", Group: "camera", Description: "The webcam drawn in text, each character's density and colour from the picture (native resolution)"})
	for _, s := range feeds.Presets {
		out = append(out, ProgramInfo{Name: "newsreel:" + s.Key, Group: "news", Description: s.Label + " as a newsreel"})
	}
	for _, s := range scenes.List() {
		out = append(out, ProgramInfo{Name: s.Name, Group: s.Group, Description: s.Description})
	}
	for _, m := range e.Media() {
		out = append(out, ProgramInfo{Name: "media:" + m, Group: "media", Description: m})
	}
	return out
}

// ProgramInfo describes one playable program.
type ProgramInfo struct {
	Name        string `json:"name"`
	Group       string `json:"group"`
	Description string `json:"description"`
}

// Media lists uploaded media files.
func (e *Engine) Media() []string {
	entries, _ := os.ReadDir(e.paths.Media)
	var out []string
	for _, d := range entries {
		if !d.IsDir() && !strings.HasPrefix(d.Name(), ".") {
			out = append(out, d.Name())
		}
	}
	sort.Strings(out)
	return out
}

func (e *Engine) makeScene(name string, seconds float64) (Program, error) {
	if name == "interference" {
		return &interferenceProgram{v: dashboard.NewInterference(), seconds: seconds}, nil
	}
	if name == "bigclock" {
		return &fullRes{name: name, v: dashboard.NewBigClock(), seconds: seconds}, nil
	}
	if name == "worldclock" {
		z := dashboard.LoadZones(filepath.Join(e.paths.Dir, "world-clock.txt"))
		return &fullRes{name: name, v: dashboard.NewWorldClock(z), seconds: seconds}, nil
	}
	if name == "weather" {
		return &fullRes{name: name, v: dashboard.NewWeather(e.weatherPlace), seconds: seconds}, nil
	}
	if name == "ascii" {
		return &fullRes{name: name, v: dashboard.NewASCII(), seconds: seconds}, nil
	}
	if name == "newsreel" || strings.HasPrefix(name, "newsreel:") {
		src := e.newsreelSource
		if key, ok := strings.CutPrefix(name, "newsreel:"); ok {
			if _, ok := feeds.LookupSource(key); !ok || strings.Contains(key, "/") {
				return nil, fmt.Errorf("unknown program %q", name)
			}
			src = func() string { return key }
		}
		return &fullRes{name: name, v: dashboard.NewNewsreel(src), seconds: seconds}, nil
	}
	switch name {
	case "flights":
		return &fullRes{name: name, v: dashboard.NewFlights(e.weatherPlace), seconds: seconds}, nil
	case "quakes":
		return &fullRes{name: name, v: dashboard.NewQuakes(), seconds: seconds}, nil
	case "onthisday":
		return &fullRes{name: name, v: dashboard.NewOnThisDay(), seconds: seconds}, nil
	case "moon":
		return &fullRes{name: name, v: dashboard.NewMoon(), seconds: seconds}, nil
	case "markets":
		path := filepath.Join(e.paths.Dir, "markets.txt")
		return &fullRes{name: name, v: dashboard.NewMarkets(func() dashboard.MarketsConfig { return dashboard.LoadMarkets(path) }), seconds: seconds}, nil
	case "calendar":
		return &fullRes{name: name, v: dashboard.NewCalendar(e.calendarArea), seconds: seconds}, nil
	case "air":
		return &fullRes{name: name, v: dashboard.NewAirQuality(e.weatherPlace), seconds: seconds}, nil
	case "warroom":
		return &fullRes{name: name, v: dashboard.NewWarRoom(), seconds: seconds}, nil
	case "sun":
		return &fullRes{name: name, v: dashboard.NewSun(e.weatherPlace), seconds: seconds}, nil
	case "claude":
		return &fullRes{name: name, v: dashboard.NewClaudeCode(), seconds: seconds}, nil
	case "claudelive":
		return &fullRes{name: name, v: dashboard.NewClaudeLive(), seconds: seconds}, nil
	case "widgets":
		return &fullRes{name: name, v: dashboard.NewWidgets(e.widgetViews), seconds: seconds}, nil
	case "claudehive":
		return &fullRes{name: name, v: dashboard.NewClaudeHive(), seconds: seconds}, nil
	case "claudecity":
		return &fullRes{name: name, v: dashboard.NewClaudeCity(), seconds: seconds}, nil
	case "claudeflow":
		return &fullRes{name: name, v: dashboard.NewClaudeFlow(), seconds: seconds}, nil
	case "claudeclock":
		return &fullRes{name: name, v: dashboard.NewClaudeClock(), seconds: seconds}, nil
	case "claudeorbit":
		return &fullRes{name: name, v: dashboard.NewClaudeOrbit(), seconds: seconds}, nil
	case "claudeburst":
		return &fullRes{name: name, v: dashboard.NewClaudeBurst(), seconds: seconds}, nil
	case "diary":
		return &fullRes{name: name, v: dashboard.NewDiary(e.weatherPlace), seconds: seconds}, nil
	case "commonplace", "naturediary", "newsdiary":
		book := map[string]string{"commonplace": "quotes", "naturediary": "nature", "newsdiary": "papers"}[name]
		return &fullRes{name: name, v: dashboard.NewDiaryBook(book, e.weatherPlace), seconds: seconds}, nil
	case "claudediary":
		return &fullRes{name: name, v: dashboard.NewClaudeDiary(), seconds: seconds}, nil
	case "claudelimits":
		return &fullRes{name: name, v: dashboard.NewClaudeLimits(), seconds: seconds}, nil
	case "claudemeter":
		return &fullRes{name: name, v: dashboard.NewClaudeMeter(), seconds: seconds}, nil
	case "progress":
		return &fullRes{name: name, v: dashboard.NewProgress(), seconds: seconds}, nil
	case "pomodoro":
		path := filepath.Join(e.paths.Dir, "focus.txt")
		return &fullRes{name: name, v: dashboard.NewFocus(func() dashboard.FocusConfig { return dashboard.LoadFocus(path) }), seconds: seconds}, nil
	case "daynight":
		zpath := filepath.Join(e.paths.Dir, "world-clock.txt")
		return &fullRes{name: name, v: dashboard.NewDayNight(e.weatherPlace, func() []dashboard.Zone { return dashboard.LoadZones(zpath) }), seconds: seconds}, nil
	case "orrery":
		return &fullRes{name: name, v: dashboard.NewOrrery(), seconds: seconds}, nil
	case "netwatch":
		return &fullRes{name: name, v: dashboard.NewNetWatch(), seconds: seconds}, nil
	case "iss":
		return &fullRes{name: name, v: dashboard.NewISS(e.weatherPlace), seconds: seconds}, nil
	case "launches":
		return &fullRes{name: name, v: dashboard.NewLaunches(), seconds: seconds}, nil
	case "rain":
		return &fullRes{name: name, v: dashboard.NewRain(e.weatherPlace), seconds: seconds}, nil
	case "status":
		path := filepath.Join(e.paths.Dir, "status.txt")
		return &fullRes{name: name, v: dashboard.NewStatus(func() []feeds.StatusPage { return dashboard.LoadStatus(path) }), seconds: seconds}, nil
	case "fx":
		path := filepath.Join(e.paths.Dir, "fx.txt")
		return &fullRes{name: name, v: dashboard.NewFX(func() dashboard.FXConfig { return dashboard.LoadFX(path) }), seconds: seconds}, nil
	case "grid":
		return &fullRes{name: name, v: dashboard.NewGridPower(e.gridOutcode), seconds: seconds}, nil
	case "aurora":
		return &fullRes{name: name, v: dashboard.NewAurora(), seconds: seconds}, nil
	case "trains":
		return &fullRes{name: name, v: dashboard.NewTrains(e.trainStation, e.trainToken), seconds: seconds}, nil
	case "wordclock":
		return &fullRes{name: name, v: dashboard.NewWordClock(), seconds: seconds}, nil
	case "flipclock":
		return &fullRes{name: name, v: dashboard.NewFlipClock(), seconds: seconds}, nil
	case "nixieclock":
		return &fullRes{name: name, v: dashboard.NewNixieClock(), seconds: seconds}, nil
	case "dialclock":
		return &fullRes{name: name, v: dashboard.NewDialClock(), seconds: seconds}, nil
	case "swarmclock":
		return &fullRes{name: name, v: dashboard.NewSwarmClock(), seconds: seconds}, nil
	case "countdown":
		return &fullRes{name: name, v: dashboard.LoadCountdown(filepath.Join(e.paths.Dir, "countdown.txt")), seconds: seconds}, nil
	}
	if name == "headlines" {
		e.startNews()
		return &fullRes{name: name, v: dashboard.NewHeadlines(e.Headlines), seconds: seconds}, nil
	}
	if name == "nowplaying" {
		e.npOnce.Do(func() { e.np = nowplaying.New() })
		return &npProgram{v: e.np, seconds: seconds}, nil
	}
	if name == "karaoke" {
		e.npOnce.Do(func() { e.np = nowplaying.New() })
		return &npProgram{v: e.np, seconds: seconds, karaoke: true}, nil
	}
	if name == "dashboard" {
		e.dashOnce.Do(func() { e.dash = dashboard.New() })
		return &dashProgram{d: e.dash, seconds: seconds}, nil
	}
	info, ok := scenes.Get(name)
	if !ok {
		return nil, fmt.Errorf("unknown program %q", name)
	}
	return &sceneProgram{name: name, scene: info.New(), seconds: seconds}, nil
}

// ErrUnknownProgram is returned by Make for names it does not know.
var ErrUnknownProgram = errors.New("unknown program")

// Make builds a program by name.
func (e *Engine) Make(name string) (Program, error) {
	switch {
	case name == "board":
		return newBoardProgram(e.paths.Messages, 0), nil
	case name == "draw":
		return e.draw, nil
	case name == "news":
		return e.newsProgram(0), nil
	case name == "hud":
		return &hudProgram{e: e}, nil
	case strings.HasPrefix(name, "media:"):
		file := filepath.Join(e.paths.Media, filepath.Base(strings.TrimPrefix(name, "media:")))
		m, err := loadMedia(file)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnknownProgram, err)
		}
		return m, nil
	}
	if name == "showcase" {
		return &showcaseProgram{e: e}, nil
	}
	if items, ok := Playlists[name]; ok {
		return &playlistProgram{name: name, items: items, per: e.opts.SceneSeconds, e: e}, nil
	}
	if pl, ok := e.customPlaylist(name); ok {
		per := pl.Seconds
		if per == 0 {
			per = e.opts.SceneSeconds
		}
		return &playlistProgram{name: name, items: pl.Items, per: per, e: e}, nil
	}
	p, err := e.makeScene(name, 0)
	if err != nil {
		return nil, fmt.Errorf("%w %q", ErrUnknownProgram, name)
	}
	return p, nil
}

// Play switches program.
func (e *Engine) Play(name string) error {
	p, err := e.Make(name)
	if err != nil {
		return err
	}
	e.setProgram(p, name)
	return nil
}

func (e *Engine) setProgram(p Program, name string) {
	e.mu.Lock()
	e.program, e.t0, e.state.Program = p, time.Now(), name
	e.mu.Unlock()
	e.save()
	e.publish(Event{Type: "state", Data: e.Status()})
}

// Push shows an image sent by an app until something else is played.
func (e *Engine) Push(img image.Image) {
	e.setProgram(&pushProgram{img: FitCover(img)}, "push")
}

// PlayMedia saves an uploaded file and plays it.
func (e *Engine) PlayMedia(filename string, data []byte) (string, error) {
	m, err := decodeMedia(filename, data)
	if err != nil {
		return "", err
	}
	safe := sanitize(filename)
	if err := os.WriteFile(filepath.Join(e.paths.Media, safe), data, 0o644); err != nil {
		return "", err
	}
	m.name = "media:" + safe
	e.setProgram(m, m.name)
	return m.name, nil
}

// DeleteMedia removes an uploaded file.
func (e *Engine) DeleteMedia(name string) error {
	return os.Remove(filepath.Join(e.paths.Media, filepath.Base(name)))
}

// Draw updates the drawing pad and makes it the current program.
func (e *Engine) Draw(img image.Image) {
	e.draw.SetImage(img)
	e.mu.Lock()
	isDraw := e.state.Program == "draw"
	e.mu.Unlock()
	if !isDraw {
		e.setProgram(e.draw, "draw")
	}
}

// Flash shows an alert over whatever is playing, then returns to it.
func (e *Engine) Flash(text string, seconds float64, colour led.RGB) {
	e.setOverlay(newFlash(text, seconds, colour))
}

// Timer starts a countdown overlay.
func (e *Engine) Timer(seconds float64, label string) {
	e.setOverlay(&timerProgram{c: scenes.NewCountdown(seconds, label)})
}

// ClearOverlay cancels a flash or timer.
func (e *Engine) ClearOverlay() { e.setOverlay(nil) }

func (e *Engine) setOverlay(p Program) {
	e.mu.Lock()
	e.overlay, e.ovT0 = p, time.Now()
	e.mu.Unlock()
	name := ""
	if p != nil {
		name = p.Name()
	}
	e.publish(Event{Type: "overlay", Data: map[string]string{"overlay": name}})
}

// SetBrightness sets software brightness (5-100).
func (e *Engine) SetBrightness(pct int) int {
	pct = max(5, min(pct, 100))
	e.mu.Lock()
	e.state.Brightness = pct
	e.mu.Unlock()
	e.save()
	e.publish(Event{Type: "state", Data: e.Status()})
	return pct
}

// SetPaused stops or resumes sending frames.
func (e *Engine) SetPaused(p bool) {
	e.mu.Lock()
	e.state.Paused = p
	e.mu.Unlock()
	e.save()
	e.publish(Event{Type: "state", Data: e.Status()})
}

// SetFlip turns the image upside down relative to the model default.
func (e *Engine) SetFlip(f bool) {
	e.mu.Lock()
	e.state.Flip = f
	if e.panel != nil {
		e.panel.Flip = f
	}
	e.mu.Unlock()
	e.save()
	e.publish(Event{Type: "state", Data: e.Status()})
}

// Status returns a snapshot.
func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := Status{State: e.state, FPS: round1(e.fps), Panel: e.panelMode, Error: e.errText, Timing: e.timing}
	if e.overlay != nil {
		s.Overlay = e.overlay.Name()
	}
	if e.panel != nil {
		s.PanelName = e.panel.Model.Name
	}
	return s
}

// PanelInfo describes the connected panel, if any.
func (e *Engine) PanelInfo() map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	info := map[string]any{"status": e.panelMode, "width": led.W, "height": led.H, "ledCols": led.Cols, "ledRows": led.Rows}
	if e.panel != nil {
		info["model"] = e.panel.Model.Name
		info["modelId"] = e.panel.Info.Model
		info["deviceId"] = fmt.Sprintf("%08x", e.panel.Info.DeviceID)
	}
	return info
}

// Preview returns the latest frame as JPEG.
func (e *Engine) Preview() []byte {
	e.previewMu.Lock()
	defer e.previewMu.Unlock()
	if e.lastFrame != nil && e.encSeq != e.frameSeq {
		var buf bytes.Buffer
		_ = jpeg.Encode(&buf, e.lastFrame, &jpeg.Options{Quality: 88})
		e.preview, e.encSeq = buf.Bytes(), e.frameSeq
	}
	return e.preview
}

// Subscribe returns a channel of events; call the returned func to unsubscribe.
func (e *Engine) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 16)
	e.subsMu.Lock()
	e.subs[ch] = struct{}{}
	e.subsMu.Unlock()
	return ch, func() {
		e.subsMu.Lock()
		if _, ok := e.subs[ch]; ok {
			delete(e.subs, ch)
			close(ch)
		}
		e.subsMu.Unlock()
	}
}

func (e *Engine) publish(ev Event) {
	e.subsMu.Lock()
	defer e.subsMu.Unlock()
	for ch := range e.subs {
		select {
		case ch <- ev:
		default: // slow subscriber: drop
		}
	}
}

func (e *Engine) save() {
	e.mu.Lock()
	s := e.state
	e.mu.Unlock()
	if s.Program == "push" {
		return
	}
	data, _ := json.MarshalIndent(s, "", "  ")
	_ = os.WriteFile(e.paths.State, data, 0o644)
}

// Run renders and sends frames until Stop.
func (e *Engine) Run() {
	defer close(e.done)
	defer e.closePanel()
	e.usb = make(chan []byte, 1)
	usbDone := make(chan struct{})
	go e.usbLoop(usbDone)
	go e.relayLoop()
	defer func() { close(e.usb); <-usbDone }()
	count, window := 0, time.Now()
	last := time.Now()
	for {
		select {
		case <-e.stop:
			return
		default:
		}
		start := time.Now()
		e.runSchedule()

		e.mu.Lock()
		p, t0 := e.program, e.t0
		_, inHUD := e.program.(*hudProgram)
		if e.overlay != nil {
			p, t0 = e.overlay, e.ovT0
		}
		bright, paused, mirror := e.state.Brightness, e.state.Paused, e.state.Mirror
		if inHUD && mirror == "" {
			mirror = hudMirror(e.state.HUDMirror)
		}
		e.mu.Unlock()

		dt := time.Since(last).Seconds()
		last = time.Now()
		img := e.safeFrame(p, time.Since(t0).Seconds(), dt)
		if img == nil {
			e.mu.Lock()
			if p == e.overlay {
				e.overlay = nil
				e.mu.Unlock()
				e.publish(Event{Type: "overlay", Data: map[string]string{"overlay": ""}})
			} else {
				e.t0 = time.Now() // finished programs loop
				e.mu.Unlock()
			}
			continue
		}
		if f, ok := p.(*flashProgram); ok && inHUD {
			img = hud.Render(hud.Info{Now: time.Now(), Alert: f.text}) // alerts in HUD style
		}
		img = hud.Mirrored(img, mirror)
		if bright < 100 {
			img = dim(img, bright)
		}
		e.updatePreview(img)
		if !paused {
			e.send(img)
		} else {
			time.Sleep(100 * time.Millisecond)
		}
		count++
		if el := time.Since(window); el >= 2*time.Second {
			e.mu.Lock()
			e.fps = float64(count) / el.Seconds()
			e.timing = e.acc.take(e.quality)
			e.mu.Unlock()
			count, window = 0, time.Now()
		}
		if iv := p.Interval(); iv > 0 {
			panel.Pace(start, iv)
		} else if e.panelMode != "connected" {
			panel.Pace(start, 83*time.Millisecond) // ~12 fps without a panel
		}
	}
}

func (e *Engine) safeFrame(p Program, t, dt float64) (img *image.RGBA) {
	defer func() {
		if r := recover(); r != nil {
			e.setError(fmt.Sprintf("%s: %v", p.Name(), r))
			e.log.Error("program panicked", "program", p.Name(), "panic", r)
			time.Sleep(500 * time.Millisecond)
			img = nil
		}
	}()
	return p.Frame(t, dt)
}

func (e *Engine) setError(s string) {
	e.mu.Lock()
	changed := e.errText != s
	e.errText = s
	e.mu.Unlock()
	if changed && s != "" {
		e.publish(Event{Type: "error", Data: map[string]string{"error": s}})
	}
}

// updatePreview records a copy of the frame (programs draw the next frame into the same
// buffer, so keeping the pointer would tear); it is only JPEG-encoded when someone asks.
func (e *Engine) updatePreview(img *image.RGBA) {
	e.previewMu.Lock()
	if e.lastFrame == nil || e.lastFrame.Bounds() != img.Bounds() {
		e.lastFrame = image.NewRGBA(img.Bounds())
	}
	draw.Draw(e.lastFrame, img.Bounds(), img, img.Bounds().Min, draw.Src)
	e.frameSeq++
	e.previewMu.Unlock()
}

// Frame size budget. Busy scenes (fire, plasma) reach 800-900 KB at quality 88 and still run
// at 12+ fps now that encoding overlaps the USB send. Only frames above 1 MB (noisy photos)
// step quality down, and it recovers when frames get simpler again.
const (
	maxQuality  = 88
	minQuality  = 70
	frameBudget = 1 << 20
)

func (e *Engine) send(img *image.RGBA) {
	if e.opts.NoPanel {
		return
	}
	e.mu.Lock()
	p, next := e.panel, e.nextTry
	e.mu.Unlock()
	if p == nil {
		if time.Now().Before(next) {
			return
		}
		np, err := panel.Open()
		if err != nil {
			e.mu.Lock()
			e.nextTry = time.Now().Add(2 * time.Second)
			e.mu.Unlock()
			e.setPanel("disconnected", fmt.Sprintf("panel: %v", err))
			return
		}
		e.mu.Lock()
		np.Flip = e.state.Flip
		e.panel = np
		e.mu.Unlock()
		p = np
		e.setPanel("connected", "")
		e.log.Info("panel connected", "model", np.Model.Name, "id", fmt.Sprintf("%08x", np.Info.DeviceID))
	}
	e.mu.Lock()
	flip := p.Model.Rotate180 != p.Flip
	if e.quality == 0 {
		e.quality = maxQuality
	}
	q := e.quality
	e.mu.Unlock()
	start := time.Now()
	var src image.Image = img
	if flip {
		src = panel.Rotate180(img)
	}
	data, err := panel.EncodeJPEG(src, q)
	if err != nil {
		e.log.Warn("encode failed", "err", err)
		return
	}
	e.mu.Lock()
	e.acc.encode(time.Since(start), len(data))
	switch {
	case len(data) > frameBudget && e.quality > minQuality:
		e.quality = max(minQuality, e.quality-4)
	case len(data) < frameBudget*3/4 && e.quality < maxQuality:
		e.quality++
	}
	e.mu.Unlock()
	select {
	case e.usb <- data:
	case <-e.stop:
	}
}

// usbLoop sends encoded frames until the channel closes.
func (e *Engine) usbLoop(done chan struct{}) {
	defer close(done)
	for data := range e.usb {
		e.mu.Lock()
		p := e.panel
		e.mu.Unlock()
		if p == nil {
			continue
		}
		start := time.Now()
		err := p.SendJPEG(data)
		d := time.Since(start)
		e.mu.Lock()
		e.acc.send(d)
		e.mu.Unlock()
		if d > 400*time.Millisecond {
			e.log.Warn("slow frame", "ms", d.Milliseconds(), "kb", len(data)>>10)
		}
		if err != nil {
			e.log.Warn("send failed; reconnecting", "err", err)
			e.closePanel()
			e.mu.Lock()
			e.nextTry = time.Now().Add(time.Second)
			e.mu.Unlock()
			e.setPanel("disconnected", fmt.Sprintf("panel: %v", err))
		}
	}
}

type timingAcc struct {
	n, ns            int
	enc, snd, sndMax time.Duration
	bytes            int
}

func (a *timingAcc) encode(d time.Duration, n int) { a.n++; a.enc += d; a.bytes += n }
func (a *timingAcc) send(d time.Duration) {
	a.ns++
	a.snd += d
	a.sndMax = max(a.sndMax, d)
}

func (a *timingAcc) take(q int) *Timing {
	if a.n == 0 {
		return nil
	}
	ms := func(d time.Duration) float64 { return round1(float64(d.Microseconds()) / 1000) }
	t := &Timing{EncodeMs: ms(a.enc / time.Duration(a.n)), FrameKB: round1(float64(a.bytes) / float64(a.n) / 1024), SendMaxMs: ms(a.sndMax), Quality: q}
	if a.ns > 0 {
		t.SendMs = ms(a.snd / time.Duration(a.ns))
	}
	*a = timingAcc{}
	return t
}

func (e *Engine) setPanel(mode, errText string) {
	e.mu.Lock()
	changed := e.panelMode != mode
	e.panelMode = mode
	if mode == "connected" || errText != "" {
		e.errText = errText
	}
	e.mu.Unlock()
	if changed {
		e.publish(Event{Type: "panel", Data: e.PanelInfo()})
	}
}

func (e *Engine) closePanel() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.panel != nil {
		e.panel.Close()
		e.panel = nil
	}
}

// Stop ends Run and releases the panel.
func (e *Engine) Stop() {
	select {
	case <-e.stop:
	default:
		close(e.stop)
	}
	<-e.done
}

func dim(img *image.RGBA, pct int) *image.RGBA {
	out := image.NewRGBA(img.Bounds())
	k := uint32(pct) * 256 / 100
	for i := 0; i < len(img.Pix); i += 4 {
		out.Pix[i] = uint8(uint32(img.Pix[i]) * k >> 8)
		out.Pix[i+1] = uint8(uint32(img.Pix[i+1]) * k >> 8)
		out.Pix[i+2] = uint8(uint32(img.Pix[i+2]) * k >> 8)
		out.Pix[i+3] = 255
	}
	return out
}

func sanitize(name string) string {
	name = filepath.Base(name)
	b := []byte(name)
	for i, c := range b {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == '_') {
			b[i] = '_'
		}
	}
	s := string(b)
	if len(s) > 80 {
		s = s[len(s)-80:]
	}
	if s == "" || s == "." || s == ".." {
		s = "upload"
	}
	return s
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

// RenderOffline runs a program without a panel for the given simulated time at 12 fps and
// returns the last frame.
func RenderOffline(dataDir, name string, seconds float64) (*image.RGBA, error) {
	paths, err := NewPaths(dataDir)
	if err != nil {
		return nil, err
	}
	e, err := New(Options{Paths: paths, NoPanel: true, Log: slog.New(slog.DiscardHandler)})
	if err != nil {
		return nil, err
	}
	p, err := e.Make(name)
	if err != nil {
		return nil, err
	}
	const dt = 1.0 / 12
	var last *image.RGBA
	for t := 0.0; t <= seconds; t += dt {
		if img := p.Frame(t, dt); img != nil {
			last = img
		}
	}
	if last == nil {
		return nil, fmt.Errorf("%s rendered nothing", name)
	}
	return last, nil
}
