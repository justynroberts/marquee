package engine

import (
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/justynroberts/marquee/internal/hud"
	"github.com/justynroberts/marquee/internal/nowplaying"
)

// hudProgram is the heads-up screen: clock, plus the HUD message, the playing track or the date.
type hudProgram struct{ e *Engine }

func (p *hudProgram) Name() string { return "hud" }
func (p *hudProgram) Interval() time.Duration {
	p.e.mu.Lock()
	defer p.e.mu.Unlock()
	if p.e.state.HUDLyrics {
		return 0 // the karaoke sweep wants every frame
	}
	return 250 * time.Millisecond
}

func (p *hudProgram) Frame(t, dt float64) *image.RGBA {
	e := p.e
	e.mu.Lock()
	msg, lyrics := e.state.HUDText, e.state.HUDLyrics
	e.mu.Unlock()
	e.npOnce.Do(func() { e.np = nowplaying.New() })
	if lyrics {
		now := time.Now()
		if cur, next, sweep, ok := e.np.Lyric(now); ok {
			return hud.Render(hud.Info{Now: now, Lyrics: true, Lyric: cur, NextLyric: next, Sweep: sweep})
		}
	}
	track := ""
	if title, artist, playing := e.np.Current(); playing && title != "" {
		track = title
		if artist != "" {
			track += " — " + artist
		}
	}
	return hud.Render(hud.Info{Now: time.Now(), Message: msg, Track: track})
}

// Mirror values.
var mirrors = map[string]string{"": "", "off": "", "none": "", "horizontal": "horizontal", "h": "horizontal", "vertical": "vertical", "v": "vertical"}

// SetMirror flips the panel output: "horizontal", "vertical" or "off".
func (e *Engine) SetMirror(how string) error {
	m, ok := mirrors[strings.ToLower(strings.TrimSpace(how))]
	if !ok {
		return fmt.Errorf("mirror must be horizontal, vertical or off")
	}
	e.mu.Lock()
	e.state.Mirror = m
	e.mu.Unlock()
	e.save()
	e.publish(Event{Type: "state", Data: e.Status()})
	return nil
}

// SetHUDText sets the HUD's message line ("" clears it).
func (e *Engine) SetHUDText(text string) {
	text = strings.Join(strings.Fields(text), " ")
	if r := []rune(text); len(r) > 120 {
		text = string(r[:120])
	}
	e.mu.Lock()
	e.state.HUDText = text
	e.mu.Unlock()
	e.save()
	e.publish(Event{Type: "state", Data: e.Status()})
}

// hudMirror resolves the HUD's mirror setting: horizontal unless set otherwise.
func hudMirror(m string) string {
	switch m {
	case "off":
		return ""
	case "vertical":
		return "vertical"
	}
	return "horizontal"
}

// SetHUDMirror sets how the HUD is flipped: "horizontal" (panel facing the screen),
// "vertical" (panel lying below the screen, facing up) or "off".
func (e *Engine) SetHUDMirror(how string) error {
	m, ok := mirrors[strings.ToLower(strings.TrimSpace(how))]
	if !ok {
		return fmt.Errorf("mirror must be horizontal, vertical or off")
	}
	if m == "" {
		m = "off"
	}
	e.mu.Lock()
	e.state.HUDMirror = m
	e.mu.Unlock()
	e.save()
	e.publish(Event{Type: "state", Data: e.Status()})
	return nil
}

// HUDMirror returns the mirror the HUD uses: horizontal, vertical or off.
func (e *Engine) HUDMirror() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if m := hudMirror(e.state.HUDMirror); m != "" {
		return m
	}
	return "off"
}

// HUDOn reports whether HUD mode is on (the hud program is playing).
func (e *Engine) HUDOn() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state.Program == "hud"
}

// SetHUD turns HUD mode on (play the HUD, which is always mirrored per HUDMirror) or off
// (back to what was playing before).
func (e *Engine) SetHUD(on bool) {
	e.mu.Lock()
	cur, prev := e.state.Program, e.state.HUDPrev
	e.mu.Unlock()
	if on {
		if cur != "hud" {
			e.mu.Lock()
			e.state.HUDPrev = cur
			e.mu.Unlock()
		}
		_ = e.Play("hud")
		return
	}
	if cur == "hud" {
		if prev == "" || prev == "hud" || e.Play(prev) != nil {
			_ = e.Play("show")
		}
	}
}

// SetLyricsLead sets how many seconds early lyric lines appear and returns the value applied.
func (e *Engine) SetLyricsLead(sec float64) float64 {
	nowplaying.SetLead(sec)
	v := nowplaying.Lead()
	e.mu.Lock()
	e.state.LyricsLead = &v
	e.mu.Unlock()
	e.save()
	return v
}

// SetHUDLyrics shows synced lyrics on the HUD (instead of the clock) while a song has them.
func (e *Engine) SetHUDLyrics(on bool) {
	e.mu.Lock()
	e.state.HUDLyrics = on
	e.mu.Unlock()
	e.save()
	e.publish(Event{Type: "state", Data: e.Status()})
}
