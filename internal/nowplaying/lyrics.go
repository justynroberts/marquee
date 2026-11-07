package nowplaying

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
)

// Synced lyrics come from LRCLIB (https://lrclib.net), a free, open lyrics database with no
// key. It works for Music and Spotify alike; nothing is sent but the track's title, artist,
// album and length.

type lyricLine struct {
	t    float64 // seconds
	text string
}

// Lead shows each line this many seconds before its timestamp. It covers the player query
// (~0.2 s), getting a frame onto the panel (~0.2 s) and the moment it takes to read a line.
// Adjustable with SetLead, since output latency (Bluetooth, AirPlay) differs per setup.
var lead atomic.Uint64

const DefaultLead = 1.1

func init() { SetLead(DefaultLead) }

// SetLead sets how early lyric lines appear, in seconds (-5..5).
func SetLead(sec float64) { lead.Store(math.Float64bits(max(-5, min(5, sec)))) }

// Lead returns the current lyric lead in seconds.
func Lead() float64 { return math.Float64frombits(lead.Load()) }

var lrcTimeRE = regexp.MustCompile(`\[(\d+):(\d+(?:\.\d+)?)\]`)

// parseLRC reads "[mm:ss.xx] text" lines (several stamps per line allowed), sorted by time.
func parseLRC(s string) []lyricLine {
	var out []lyricLine
	for _, raw := range strings.Split(s, "\n") {
		raw = strings.TrimRight(raw, "\r")
		stamps := lrcTimeRE.FindAllStringSubmatchIndex(raw, -1)
		if len(stamps) == 0 || stamps[0][0] != 0 {
			continue
		}
		text := strings.TrimSpace(raw[stamps[len(stamps)-1][1]:])
		for _, m := range stamps {
			mm, _ := strconv.Atoi(raw[m[2]:m[3]])
			ss, _ := strconv.ParseFloat(raw[m[4]:m[5]], 64)
			out = append(out, lyricLine{t: float64(mm)*60 + ss, text: text})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].t < out[j].t })
	return out
}

// lineAt returns the index of the line showing at pos, or -1 before the first.
func lineAt(lines []lyricLine, pos float64) int {
	pos += Lead()
	return sort.Search(len(lines), func(i int) bool { return lines[i].t > pos }) - 1
}

// LyricsBase is the LRCLIB API root (tests point it elsewhere).
var LyricsBase = "https://lrclib.net/api"

var errNoLyrics = errors.New("no synced lyrics")

type lrclibRecord struct {
	SyncedLyrics string  `json:"syncedLyrics"`
	Instrumental bool    `json:"instrumental"`
	Duration     float64 `json:"duration"`
}

var fetchLyrics = fetchLRCLIB

func fetchLRCLIB(ctx context.Context, t Track) ([]lyricLine, error) {
	if t.Title == "" || t.Artist == "" {
		return nil, errNoLyrics
	}
	q := url.Values{"track_name": {t.Title}, "artist_name": {t.Artist}}
	if t.Album != "" {
		q.Set("album_name", t.Album)
	}
	if t.Duration > 0 {
		q.Set("duration", strconv.Itoa(int(math.Round(t.Duration))))
	}
	var rec lrclibRecord
	found, err := lrclibGet(ctx, "/get?"+q.Encode(), &rec)
	if err != nil {
		return nil, err
	}
	if !found || rec.SyncedLyrics == "" {
		// The exact match needs album and length to agree; search is looser. Take the closest
		// length among results that have synced lyrics.
		var recs []lrclibRecord
		sq := url.Values{"track_name": {t.Title}, "artist_name": {t.Artist}}
		if _, err := lrclibGet(ctx, "/search?"+sq.Encode(), &recs); err != nil {
			return nil, err
		}
		best := -1
		for i, r := range recs {
			if r.SyncedLyrics == "" {
				continue
			}
			if t.Duration > 0 && math.Abs(r.Duration-t.Duration) > 4 {
				continue
			}
			if best < 0 || math.Abs(r.Duration-t.Duration) < math.Abs(recs[best].Duration-t.Duration) {
				best = i
			}
		}
		if best < 0 {
			return nil, errNoLyrics
		}
		rec = recs[best]
	}
	lines := parseLRC(rec.SyncedLyrics)
	if len(lines) == 0 {
		return nil, errNoLyrics
	}
	return lines, nil
}

func lrclibGet(ctx context.Context, path string, v any) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, LyricsBase+path, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", "Marquee (https://github.com/justynroberts/marquee)")
	resp, err := lyricsClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("lrclib: HTTP %d", resp.StatusCode)
	}
	return true, json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(v)
}

var lyricsClient = &http.Client{Timeout: 8e9}
