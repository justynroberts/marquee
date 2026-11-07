package nowplaying

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // artwork decoders
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Track is what one player reports.
type Track struct {
	Source   string // "Music" or "Spotify"
	ID       string // persistent ID (Music) or spotify:track URI
	Title    string
	Artist   string
	Album    string
	Position float64 // seconds
	Duration float64 // seconds; 0 for streams without a length
	Playing  bool    // false when paused
	ArtURL   string  // Spotify only
}

func (t Track) key() string {
	if t.ID != "" {
		return t.Source + "\x1f" + t.ID
	}
	return strings.Join([]string{t.Source, t.Title, t.Artist, t.Album}, "\x1f")
}

// playerError is a per-app AppleScript error (e.g. -1743 when Automation is not allowed).
type playerError struct {
	Source string
	Code   int
	Msg    string
}

func (e playerError) denied() bool { return e.Code == -1743 || e.Code == -1744 }

// poll is the result of one query of both players.
type poll struct {
	Tracks []Track
	Errs   []playerError
}

const (
	fieldSep  = "\x1f" // ASCII unit separator
	recordSep = "\x1e" // ASCII record separator
)

// playerScript queries one player. It is compiled only at run time (via run script) and only
// after the player is known to be running, so a player that is not installed or not running is
// neither launched nor needed to compile the script. Numbers leave as integer milliseconds so the
// output is locale-independent. Fields: T, source, state, title, artist, album, position ms,
// duration ms, artwork URL, track ID.
const playerScript = `on toStr(x)
	if x is missing value then return ""
	return x as string
end toStr
on toMs(x)
	try
		return (x * 1000) as integer
	on error
		return 0
	end try
end toMs
set sepU to character id 31
tell application "%[1]s"
	set pstate to player state as string
	if pstate is not "playing" and pstate is not "paused" then return ""
	set t to current track
	return "T" & sepU & "%[1]s" & sepU & pstate & sepU & my toStr(name of t) & sepU & my toStr(artist of t) & sepU & my toStr(album of t) & sepU & my toMs(player position) & sepU & %[2]s & sepU & %[3]s & sepU & my toStr(%[4]s of t)
end tell`

// Per-player differences: Music reports duration in seconds and has no artwork URL; Spotify
// reports duration in ms.
var (
	musicScript   = fmt.Sprintf(playerScript, "Music", "my toMs(duration of t)", `""`, "persistent ID")
	spotifyScript = fmt.Sprintf(playerScript, "Spotify", "my toStr(duration of t)", "my toStr(artwork url of t)", "id")
)

// asString quotes s as an AppleScript string literal.
func asString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func guarded(app, inner string) string {
	return fmt.Sprintf(`if application "%[1]s" is running then
	try
		set r to run script %[2]s
		if r is not "" then set out to out & r & sepR
	on error m number n
		set out to out & "E" & sepU & "%[1]s" & sepU & n & sepU & m & sepR
	end try
end if
`, app, asString(inner))
}

var pollScript = "set sepU to character id 31\nset sepR to character id 30\nset out to \"\"\n" +
	guarded("Music", musicScript) + guarded("Spotify", spotifyScript) + "return out"

// fetch queries the players. Tests replace it.
var fetch = fetchOsascript

// fetchArt returns the current track's artwork (nil, nil when it has none). Tests replace it.
var fetchArt = fetchArtwork

var errCodeRE = regexp.MustCompile(`\((-?\d+)\)\s*$`)

func runOsascript(ctx context.Context, script string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", errors.New("osascript timed out")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New(msg)
	}
	return strings.TrimRight(stdout.String(), "\r\n"), nil
}

func fetchOsascript(ctx context.Context) (poll, error) {
	out, err := runOsascript(ctx, pollScript)
	if err != nil {
		// A whole-script failure; recover an error number if osascript printed one.
		pe := playerError{Source: "Music", Msg: err.Error()}
		if m := errCodeRE.FindStringSubmatch(err.Error()); m != nil {
			pe.Code, _ = strconv.Atoi(m[1])
		}
		if strings.Contains(err.Error(), "Spotify") {
			pe.Source = "Spotify"
		}
		return poll{Errs: []playerError{pe}}, nil
	}
	return parse(out), nil
}

// parse reads the record/unit-separated script output.
func parse(out string) poll {
	var p poll
	for _, rec := range strings.Split(out, recordSep) {
		rec = strings.TrimRight(rec, "\r\n")
		if rec == "" {
			continue
		}
		f := strings.Split(rec, fieldSep)
		switch f[0] {
		case "T":
			if len(f) < 10 {
				continue
			}
			t := Track{
				Source:  f[1],
				Playing: f[2] == "playing",
				Title:   f[3], Artist: f[4], Album: f[5],
				Position: msToSec(f[6]),
				Duration: msToSec(f[7]),
				ArtURL:   f[8],
				ID:       f[9],
			}
			// Some Spotify builds report player position in ms rather than seconds.
			if t.Duration > 0 && t.Position > t.Duration+5 && t.Position/1000 <= t.Duration {
				t.Position /= 1000
			}
			if t.Title == "" && t.Artist == "" && t.Album == "" {
				continue
			}
			p.Tracks = append(p.Tracks, t)
		case "E":
			if len(f) < 4 {
				continue
			}
			code, _ := strconv.Atoi(strings.TrimSpace(f[2]))
			p.Errs = append(p.Errs, playerError{Source: f[1], Code: code, Msg: strings.Join(f[3:], " ")})
		}
	}
	return p
}

func msToSec(s string) float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v / 1000
}

// pick prefers a playing track, then a paused one.
func (p poll) pick() (Track, bool) {
	for _, t := range p.Tracks {
		if t.Playing {
			return t, true
		}
	}
	if len(p.Tracks) > 0 {
		return p.Tracks[0], true
	}
	return Track{}, false
}

// idleMessage explains why nothing is shown, or "" when nothing is simply playing.
func (p poll) idleMessage() string {
	var denied []string
	other := ""
	for _, e := range p.Errs {
		if e.denied() {
			denied = append(denied, e.Source)
		} else if other == "" {
			other = e.Source + ": " + e.Msg
		}
	}
	if len(denied) > 0 {
		return "Allow Marquee to control " + strings.Join(denied, " and ") + " in System Settings › Privacy & Security › Automation"
	}
	return other
}

// ---------------------------------------------------------------- artwork

var artPath = filepath.Join(os.TempDir(), fmt.Sprintf("marquee-nowplaying-art-%d", os.Getpid()))

const musicArtScript = `set p to POSIX file %q
if application "Music" is running then
	tell application "Music"
		set t to current track
		if (count of artworks of t) is 0 then return "NONE"
		set d to raw data of artwork 1 of t
	end tell
	set f to open for access p with write permission
	try
		set eof f to 0
		write d to f
		close access f
	on error m number n
		close access f
		error m number n
	end try
	return "OK"
end if
return "NONE"`

var httpClient = &http.Client{Timeout: 4 * time.Second}

func fetchArtwork(ctx context.Context, t Track) (image.Image, error) {
	switch t.Source {
	case "Music":
		_ = os.Remove(artPath)
		out, err := runOsascript(ctx, fmt.Sprintf(musicArtScript, artPath))
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(out) != "OK" {
			return nil, nil
		}
		data, err := os.ReadFile(artPath)
		_ = os.Remove(artPath)
		if err != nil {
			return nil, err
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		return img, err
	case "Spotify":
		if t.ArtURL == "" {
			return nil, nil
		}
		ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.ArtURL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("artwork: %s", resp.Status)
		}
		img, _, err := image.Decode(io.LimitReader(resp.Body, 16<<20))
		return img, err
	}
	return nil, nil
}
