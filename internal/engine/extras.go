package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// ---------------------------------------------------------------- custom playlists

// Playlist is a named sequence of programs. Items are scene names, "board", "news",
// "media:FILE" or another program name; each plays for Seconds (scenes, media) or one pass
// (board, news).
type Playlist struct {
	Items   []string `json:"items"`
	Seconds float64  `json:"seconds,omitempty"`
	Builtin bool     `json:"builtin,omitempty"`
}

var playlistNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

func (e *Engine) playlistsFile() string { return filepath.Join(e.paths.Dir, "playlists.json") }

func (e *Engine) loadPlaylists() {
	b, err := os.ReadFile(e.playlistsFile())
	if err != nil {
		return
	}
	var m map[string]Playlist
	if json.Unmarshal(b, &m) == nil {
		e.custom = m
	}
}

// AllPlaylists returns built-in and custom playlists.
func (e *Engine) AllPlaylists() map[string]Playlist {
	out := map[string]Playlist{}
	for n, items := range Playlists {
		out[n] = Playlist{Items: items, Builtin: true}
	}
	e.mu.Lock()
	for n, p := range e.custom {
		out[n] = p
	}
	e.mu.Unlock()
	return out
}

func (e *Engine) customPlaylist(name string) (Playlist, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, ok := e.custom[name]
	return p, ok
}

// SetPlaylist creates or replaces a custom playlist after checking every item can play.
func (e *Engine) SetPlaylist(name string, p Playlist) error {
	if !playlistNameRe.MatchString(name) {
		return fmt.Errorf("playlist names are lowercase letters, digits, - and _ (up to 40)")
	}
	if _, ok := Playlists[name]; ok || name == "showcase" {
		return fmt.Errorf("%q is a built-in playlist", name)
	}
	if _, err := e.makeScene(name, 0); err == nil || reservedName(name) {
		return fmt.Errorf("%q is already a program name", name)
	}
	if len(p.Items) == 0 || len(p.Items) > 100 {
		return fmt.Errorf("a playlist needs 1 to 100 items")
	}
	for _, it := range p.Items {
		if it == name {
			return fmt.Errorf("a playlist cannot contain itself")
		}
		prog, err := e.makeItem(it, 1)
		if err != nil {
			return fmt.Errorf("item %q: %v", it, err)
		}
		if c, ok := prog.(interface{ Close() }); ok {
			c.Close()
		}
	}
	p.Seconds = max(0, min(p.Seconds, 3600))
	p.Builtin = false
	e.mu.Lock()
	if e.custom == nil {
		e.custom = map[string]Playlist{}
	}
	e.custom[name] = p
	b, _ := json.MarshalIndent(e.custom, "", "  ")
	e.mu.Unlock()
	return os.WriteFile(e.playlistsFile(), b, 0o644)
}

// DeletePlaylist removes a custom playlist.
func (e *Engine) DeletePlaylist(name string) error {
	e.mu.Lock()
	if _, ok := e.custom[name]; !ok {
		e.mu.Unlock()
		return fmt.Errorf("%w %q", ErrUnknownProgram, name)
	}
	delete(e.custom, name)
	b, _ := json.MarshalIndent(e.custom, "", "  ")
	e.mu.Unlock()
	return os.WriteFile(e.playlistsFile(), b, 0o644)
}

func reservedName(n string) bool {
	switch n {
	case "board", "draw", "news", "push", "showcase", "nowplaying", "karaoke", "hud", "dashboard", "interference":
		return true
	}
	return strings.HasPrefix(n, "media:")
}

// makeItem builds one playlist entry: board and news run one pass, everything else runs for
// per seconds (0 = its own length, or forever for scenes).
func (e *Engine) makeItem(item string, per float64) (Program, error) {
	switch {
	case item == "board":
		return newBoardProgram(e.paths.Messages, 1), nil
	case item == "news":
		return e.newsProgram(1), nil
	case strings.HasPrefix(item, "media:"):
		m, err := e.Make(item)
		if err != nil {
			return nil, err
		}
		return &timed{Program: m, seconds: per}, nil
	}
	if p, err := e.makeScene(item, per); err == nil {
		return p, nil
	}
	if _, ok := Playlists[item]; ok || item == "showcase" {
		return nil, fmt.Errorf("playlists cannot contain playlists")
	}
	if _, ok := e.customPlaylist(item); ok {
		return nil, fmt.Errorf("playlists cannot contain playlists")
	}
	m, err := e.Make(item)
	if err != nil {
		return nil, err
	}
	return &timed{Program: m, seconds: per}, nil
}

// timed ends a program after seconds (0 = never).
type timed struct {
	Program
	seconds float64
}

func (p *timed) Frame(t, dt float64) *image.RGBA {
	if p.seconds > 0 && t > p.seconds {
		return nil
	}
	return p.Program.Frame(t, dt)
}

func (e *Engine) playlistNames() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for n := range e.custom {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------- news ticker

// The news ticker is the message board reading news.txt, which a background fetcher rewrites
// every ten minutes from Hacker News, or from the RSS/Atom feeds listed one per line in
// news-sources.txt.

const newsEvery = 10 * time.Minute

func (e *Engine) newsFile() string { return filepath.Join(e.paths.Dir, "news.txt") }

func (e *Engine) newsProgram(passes int) Program {
	e.newsOnce.Do(func() {
		if _, err := os.Stat(e.newsFile()); err != nil {
			_ = os.WriteFile(e.newsFile(), []byte("@step=3\n[orange]News[/] [grey]fetching headlines…\n"), 0o644)
		}
		go e.newsLoop()
	})
	p := newBoardProgram(e.newsFile(), passes)
	return &named{Program: p, name: "news"}
}

type named struct {
	Program
	name string
}

func (p *named) Name() string { return p.name }

func (e *Engine) newsLoop() {
	for {
		e.RefreshNews()
		select {
		case <-e.stop:
			return
		case <-time.After(newsEvery):
		}
	}
}

// RefreshNews fetches headlines now and rewrites news.txt.
func (e *Engine) RefreshNews() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var hs []feeds.Headline
	var errs []string
	for _, src := range e.newsSources() {
		var got []feeds.Headline
		var err error
		if src == "hn" {
			got, err = feeds.FetchHN(ctx, 15)
		} else {
			got, err = feeds.FetchRSS(ctx, src, 10)
		}
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		hs = append(hs, got...)
	}
	if len(hs) == 0 {
		msg := strings.Join(errs, "; ")
		e.log.Warn("news unavailable", "err", msg)
		if _, err := os.Stat(e.newsFile()); err != nil || isPlaceholder(e.newsFile()) {
			_ = os.WriteFile(e.newsFile(), []byte("@step=3\n[orange]News[/] [grey]headlines unavailable — check the network\n"), 0o644)
		}
		return fmt.Errorf("news: %s", msg)
	}
	return os.WriteFile(e.newsFile(), []byte(feeds.BoardText(hs)), 0o644)
}

func isPlaceholder(path string) bool {
	b, _ := os.ReadFile(path)
	return strings.Contains(string(b), "[grey]fetching") || strings.Contains(string(b), "unavailable")
}

func (e *Engine) newsSources() []string {
	b, err := os.ReadFile(filepath.Join(e.paths.Dir, "news-sources.txt"))
	if err != nil {
		return []string{"hn"}
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if strings.EqualFold(l, "hn") || strings.EqualFold(l, "hackernews") {
			out = append(out, "hn")
		} else if strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		return []string{"hn"}
	}
	return out
}
