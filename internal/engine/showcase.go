package engine

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"time"

	"github.com/justynroberts/marquee/internal/led"
	"github.com/justynroberts/marquee/internal/scenes"
)

// The showcase tours everything Marquee can do, with a title card before each part. It is an
// ordinary program: play "showcase" from the menu, the remote, the API or the schedule.

type showStep struct {
	kind    string // scene | board | flash | timer
	name    string // program or board file
	title   string
	caption string
	seconds float64
	text    string // board text, flash text or timer label
}

const showcaseBoard = `@step=4
[big][rainbow]Marquee :star:
[red]Colour [orange]in [yellow]every [green]tag [cyan]and [#ff8fd0]hex [/]
[wave]waves [/] [shimmer]shimmer [/] [pulse]pulse [/] [blink]blink
[serif]Serif [mono]Mono [script]Script [condensed]Impact [typewriter]Typewriter [pixel]Pixel
[bg=blue][white] Backgrounds [/] [bg=purple][yellow] behind [/] [bg=red] text [/]
:heart: :fire: :star: :ufo: :wave: :check: and real emoji ☕️ 🚀 🎉 👍🏽 🐙 🇬🇧
Live: {time} · {day} · CPU {cpu}% · memory {mem}%
!Alerts pulse in vermilion
`

const showcaseGreen = `@theme=green
@step=3
[big]Phosphor themes
green or amber, with glow and scanlines {time}
`

var showcaseSteps = []showStep{
	{kind: "board", name: "showcase-board.txt", title: "Message board", caption: "colours, effects, fonts, sprites, emoji, live tokens", text: showcaseBoard},
	{kind: "board", name: "showcase-green.txt", title: "Phosphor", caption: "@theme=green on the board", text: showcaseGreen},
	{kind: "scene", name: "matrix", title: "Matrix", caption: "katakana rain", seconds: 12},
	{kind: "scene", name: "terminal", title: "Terminal", caption: "this Mac, typed live", seconds: 14},
	{kind: "scene", name: "nowplaying", title: "Now Playing", caption: "Music or Spotify, with artwork", seconds: 10},
	{kind: "scene", name: "news", title: "Headlines", caption: "Hacker News or any RSS feed", seconds: 14},
	{kind: "scene", name: "dashboard", title: "Dashboard", caption: "clock, CPU, memory, network at full resolution", seconds: 10},
	{kind: "scene", name: "interference", title: "Interference", caption: "a full-resolution live stream", seconds: 10},
	{kind: "scene", name: "fire", title: "Fire", caption: "heat buffer", seconds: 9},
	{kind: "scene", name: "plasma", title: "Plasma", caption: "", seconds: 9},
	{kind: "scene", name: "life", title: "Life", caption: "Conway, coloured by age", seconds: 10},
	{kind: "scene", name: "warp", title: "Warp", caption: "", seconds: 8},
	{kind: "scene", name: "rain", title: "Rain", caption: "", seconds: 8},
	{kind: "scene", name: "fireworks", title: "Fireworks", caption: "", seconds: 10},
	{kind: "scene", name: "aquarium", title: "Aquarium", caption: "", seconds: 10},
	{kind: "scene", name: "bounce", title: "Bounce", caption: "", seconds: 8},
	{kind: "scene", name: "clock", title: "Clock", caption: "", seconds: 8},
	{kind: "scene", name: "spectrum", title: "Audio: spectrum", caption: "play some music", seconds: 9},
	{kind: "scene", name: "mirror", title: "Audio: mirror", caption: "", seconds: 9},
	{kind: "scene", name: "waterfall", title: "Audio: waterfall", caption: "", seconds: 9},
	{kind: "scene", name: "scope", title: "Audio: scope", caption: "", seconds: 9},
	{kind: "scene", name: "pulse", title: "Audio: pulse", caption: "", seconds: 9},
	{kind: "scene", name: "meter", title: "Audio: meter", caption: "", seconds: 9},
	{kind: "flash", title: "Alerts", caption: "POST /v1/flash from any app", seconds: 6, text: "Build failed 🔥"},
	{kind: "timer", title: "Timers", caption: "POST /v1/timer", seconds: 12, text: "showcase"},
}

const cardSeconds = 2.2

type showcaseProgram struct {
	e      *Engine
	i      int
	inCard bool
	t0     float64
	cur    Program
}

func (p *showcaseProgram) Name() string            { return "showcase" }
func (p *showcaseProgram) Interval() time.Duration { return 0 }

func (p *showcaseProgram) Frame(t, dt float64) *image.RGBA {
	if p.cur == nil && !p.inCard {
		p.inCard, p.t0 = true, t
	}
	for guard := 0; guard < len(showcaseSteps)+2; guard++ {
		step := showcaseSteps[p.i%len(showcaseSteps)]
		if p.inCard {
			if t-p.t0 < cardSeconds {
				return titleCard(p.i%len(showcaseSteps), step, t-p.t0)
			}
			p.inCard, p.t0 = false, t
			p.cur = p.start(step)
			if p.cur == nil {
				p.next(t)
				continue
			}
		}
		if p.cur != nil {
			if step.seconds > 0 && t-p.t0 > step.seconds {
				p.next(t)
				continue
			}
			if img := p.cur.Frame(t-p.t0, dt); img != nil {
				return img
			}
		}
		p.next(t)
	}
	return nil
}

func (p *showcaseProgram) next(t float64) {
	p.i++
	p.cur, p.inCard, p.t0 = nil, true, t
}

func (p *showcaseProgram) start(s showStep) Program {
	switch s.kind {
	case "board":
		path := filepath.Join(p.e.paths.Dir, s.name)
		_ = os.WriteFile(path, []byte(s.text), 0o644)
		return newBoardProgram(path, 1)
	case "flash":
		return newFlash(s.text, s.seconds, led.Alert)
	case "timer":
		return &timerProgram{c: scenes.NewCountdown(s.seconds-4, s.text)}
	default:
		if s.name == "news" {
			return &timed{Program: p.e.newsProgram(0), seconds: s.seconds}
		}
		prog, err := p.e.makeItem(s.name, s.seconds)
		if err != nil {
			return nil
		}
		return prog
	}
}

// fitText renders at the largest size that fits the panel width (minus margins).
func fitText(text, font string, sizes []int, top int) *led.Mask {
	var m *led.Mask
	for _, z := range sizes {
		m = led.TextAt(text, font, z, top)
		if m.W <= led.Cols-12 {
			break
		}
	}
	return m
}

// titleCard: index top left in dim ink, the name large, a one-line caption beneath, and a thin
// progress rule along the bottom. Left-weighted, no motion except the rule.
func titleCard(i int, s showStep, t float64) *image.RGBA {
	g := led.NewGrid()
	idx := led.TextAt(fmt.Sprintf("%02d / %02d", i+1, len(showcaseSteps)), "mono", 12, 4)
	g.Stamp(idx, 6, 0, led.Ink3)
	name := fitText(s.title, "sans", []int{30, 26, 22, 18}, 18)
	g.Stamp(name, 6, 0, led.Ink)
	if s.caption != "" {
		g.Stamp(fitText(s.caption, "sans", []int{12, 11, 10, 9, 8}, 50), 6, 0, led.Ink2)
	}
	w := int(float64(led.Cols-12) * min(1, t/cardSeconds))
	g.Fill(6, led.Rows-2, 6+w, led.Rows-1, led.Alert)
	return g.Image(led.Ground)
}
