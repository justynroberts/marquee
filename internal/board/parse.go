package board

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/justynroberts/marquee/internal/emoji"
	"github.com/justynroberts/marquee/internal/led"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

// Effects are bit flags so they combine.
const (
	FxRainbow uint8 = 1 << iota
	FxBlink
	FxPulse
	FxWave
	FxShimmer
)

var effectTags = map[string]uint8{"rainbow": FxRainbow, "blink": FxBlink, "pulse": FxPulse, "wave": FxWave, "shimmer": FxShimmer}
var sizeTags = map[string]int{"big": 58, "small": 26, "tiny": 14}

// Palette is the board's named colours, exactly those board.py accepts.
var Palette = map[string]led.RGB{
	"white": {R: 236, G: 232, B: 220}, "grey": {R: 140, G: 138, B: 130}, "red": {R: 255, G: 64, B: 56},
	"orange": {R: 255, G: 140, B: 40}, "yellow": {R: 255, G: 214, B: 60}, "green": {R: 70, G: 220, B: 110},
	"cyan": {R: 60, G: 210, B: 230}, "blue": {R: 80, G: 140, B: 255}, "purple": {R: 170, G: 110, B: 255},
	"pink": {R: 255, G: 105, B: 200}, "alert": {R: 255, G: 91, B: 56}, "black": {R: 0, G: 0, B: 0},
}

var hexColourRe = regexp.MustCompile(`^#?([0-9a-f]{6}|[0-9a-f]{3})$`)

// parseColour matches board.py parse_colour: a palette name, "#rrggbb", "rrggbb", "#rgb" or "rgb".
func parseColour(v string) (led.RGB, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if c, ok := Palette[v]; ok {
		return c, true
	}
	m := hexColourRe.FindStringSubmatch(v)
	if m == nil {
		return led.RGB{}, false
	}
	h := m[1]
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	n, _ := strconv.ParseUint(h, 16, 32)
	return led.RGB{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n)}, true
}

// isDigits matches Python's str.isdigit for ASCII: one or more 0-9, nothing else.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Settings are the board-wide @key=value lines.
type Settings struct {
	Step     int
	Gap      int
	Font     string
	Size     int
	Colour   led.RGB
	Dim      led.RGB
	Bg       led.RGB
	Theme    string // "green" / "amber": phosphor terminal look
	Location string // city for {weather} / {temp}
}

// DefaultSettings returns the settings used when messages.txt sets none.
func DefaultSettings() Settings {
	step := 2
	if v, err := strconv.Atoi(os.Getenv("BOARD_STEP")); err == nil && v > 0 {
		step = v
	}
	return Settings{Step: step, Gap: 34, Font: "sans", Size: 44, Colour: Palette["white"], Dim: led.RGB{R: 30, G: 30, B: 26}, Bg: led.RGB{R: 12, G: 13, B: 11}}
}

// Style is the state tags build up while parsing a message.
type Style struct {
	Colour  led.RGB
	Effects uint8
	Font    string
	Size    int
	Bg      led.RGB
	HasBg   bool
}

// Segment is a piece of a message: "text", "emoji" or "sprite".
type Segment struct {
	Kind  string
	Value string
	Style Style
}

// DefaultMessages is written to messages.txt when it does not exist.
var DefaultMessages = `# One message per line. Tags: [red] [#ff00aa] [bg=blue] [rainbow] [blink] [pulse] [wave] [shimmer]
# [serif] [script] [pixel] [font=Name] [big] [small] [size=30] ... and [/] to close.
# Sprites: :heart: :fire: :star: :ufo: :wave: :check: :cross:   Emoji work too: 🚀 🎉 ☕️
# Tokens: {time} {date} {day} {cpu} {mem} {weather} {temp} {incidents} {incident}.  A leading ! makes an alert.
@step=2
[white]{time}[/]  [cyan]{day}[/] {date}
[rainbow]Welcome to the board[/] :star:
[serif]Fonts:[/] [script]script[/] [typewriter]typewriter[/] [condensed]CONDENSED[/] [pixel]PIXEL[/] [chalk]chalk[/]
Coffee time ☕️ :heart: :heart:
CPU [yellow]{cpu}%[/]  memory [cyan]{mem}%[/]
[wave][green]Build passed[/][/] :check:   Deploy [blink][red]waiting[/][/] :fire:
[bg=blue][white] ON AIR [/][/]  [big]BIG[/] [small]and small[/]
:ufo: [purple][shimmer]Incoming[/][/] 🚀🎉
! Alerts pulse in vermilion :cross:
`

// ---------------------------------------------------------------------------
// Tokens
// ---------------------------------------------------------------------------

// Tokens provides several {token} values at once.
type Tokens func() map[string]string

var (
	tokMu      sync.RWMutex
	tokSources = map[string]func() string{}
	tokMaps    []Tokens
	locMu      sync.RWMutex
	location   string
)

// SetTokenSource registers {name}. A nil f removes it.
func SetTokenSource(name string, f func() string) {
	tokMu.Lock()
	defer tokMu.Unlock()
	if f == nil {
		delete(tokSources, name)
		return
	}
	tokSources[name] = f
}

// AddTokens registers a provider of several tokens.
func AddTokens(t Tokens) {
	tokMu.Lock()
	defer tokMu.Unlock()
	tokMaps = append(tokMaps, t)
}

// CurrentLocation is the @location of the most recently parsed messages (for weather sources).
func CurrentLocation() string {
	locMu.RLock()
	defer locMu.RUnlock()
	return location
}

func setLocation(l string) {
	locMu.Lock()
	location = l
	locMu.Unlock()
}

var tokenRe = regexp.MustCompile(`\{([a-z_]+)\}`)

// frozenTokens, when set (tests only), replaces token values so frames are reproducible.
var frozenTokens map[string]string

func fillTokens(text string) string {
	if !strings.Contains(text, "{") {
		return text
	}
	if frozenTokens != nil {
		for k, v := range frozenTokens {
			text = strings.ReplaceAll(text, "{"+k+"}", v)
		}
		return text
	}
	return tokenRe.ReplaceAllStringFunc(text, func(m string) string {
		name := m[1 : len(m)-1]
		if v, ok := builtinToken(name); ok {
			return v
		}
		tokMu.RLock()
		f := tokSources[name]
		maps := append([]Tokens(nil), tokMaps...)
		tokMu.RUnlock()
		if f != nil {
			return f()
		}
		for _, t := range maps {
			if v, ok := t()[name]; ok {
				return v
			}
		}
		return m
	})
}

func builtinToken(name string) (string, bool) {
	now := time.Now()
	switch name {
	case "time":
		return now.Format("15:04"), true
	case "date":
		return now.Format("02 Jan 2006"), true
	case "day":
		return now.Format("Monday"), true
	case "cpu":
		if p, err := cpu.Percent(0, false); err == nil && len(p) > 0 {
			return fmt.Sprintf("%.0f", p[0]), true
		}
		return "?", true
	case "mem":
		if v, err := mem.VirtualMemory(); err == nil {
			return fmt.Sprintf("%.0f", v.UsedPercent), true
		}
		return "?", true
	}
	return "", false
}

// ---------------------------------------------------------------------------
// Parsing
// ---------------------------------------------------------------------------

var (
	tagTokenRe = regexp.MustCompile(`\[[^\[\]\s]*\]|:[a-z]+:`)
	spriteRe   = regexp.MustCompile(`^:([a-z]+):$`)
)

// fontAliases are the font shortcut tags board.py accepts.
var fontAliases = map[string]bool{"sans": true, "serif": true, "mono": true, "script": true, "condensed": true,
	"rounded": true, "typewriter": true, "chalk": true, "stencil": true, "didot": true, "futura": true,
	"marker": true, "hand": true, "poster": true, "pixel": true}

func isFontAlias(name string) bool { return fontAliases[strings.ToLower(name)] }

// applyTag returns the style after opening [tag], or false if the tag is not recognised.
func applyTag(st Style, tag string) (Style, bool) {
	name, value, hasValue := strings.Cut(tag, "=")
	name = strings.ToLower(strings.TrimSpace(name))
	if !hasValue {
		if c, ok := parseColour(name); ok {
			st.Colour = c
			return st, true
		}
		if fx, ok := effectTags[name]; ok {
			st.Effects |= fx
			return st, true
		}
		if isFontAlias(name) {
			st.Font = name
			return st, true
		}
		if sz, ok := sizeTags[name]; ok {
			st.Size = sz
			return st, true
		}
		if name == "plain" || name == "noeffect" {
			st.Effects = 0
			return st, true
		}
		return st, false
	}
	value = strings.TrimSpace(value)
	switch name {
	case "colour", "color", "c":
		if c, ok := parseColour(value); ok {
			st.Colour = c
			return st, true
		}
	case "bg":
		if c, ok := parseColour(value); ok {
			st.Bg, st.HasBg = c, true
			return st, true
		}
	case "font", "f":
		if value != "" {
			st.Font = value
			return st, true
		}
	case "size", "s":
		if isDigits(value) {
			n, _ := strconv.Atoi(value)
			st.Size = max(8, min(n, led.Rows))
			return st, true
		}
	}
	return st, false
}

// parseMessage turns one message line into styled segments.
func parseMessage(line string, s Settings) []Segment {
	base := Style{Colour: s.Colour, Font: s.Font, Size: s.Size}
	if strings.HasPrefix(line, "!") {
		line = strings.TrimSpace(line[1:])
		base.Colour = Palette["alert"]
		base.Effects = FxPulse
	}
	line = fillTokens(line)
	st := base
	var stack []Style
	var segs []Segment
	emitText := func(part string) {
		for _, r := range emoji.Split(part) {
			kind := "text"
			if r.Emoji {
				kind = "emoji"
			}
			segs = append(segs, Segment{Kind: kind, Value: r.Text, Style: st})
		}
	}
	last := 0
	for _, loc := range tagTokenRe.FindAllStringIndex(line, -1) {
		if loc[0] > last {
			emitText(line[last:loc[0]])
		}
		part := line[loc[0]:loc[1]]
		last = loc[1]
		if strings.HasPrefix(part, "[") {
			inner := part[1 : len(part)-1]
			if inner == "/" || inner == "" || inner == "reset" {
				if len(stack) > 0 {
					st = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				} else {
					st = base
				}
				continue
			}
			if ns, ok := applyTag(st, inner); ok {
				stack = append(stack, st)
				st = ns
				continue
			}
			emitText(part) // unknown tag: shown as text
			continue
		}
		if m := spriteRe.FindStringSubmatch(part); m != nil {
			if _, ok := spriteSpecs[m[1]]; ok {
				segs = append(segs, Segment{Kind: "sprite", Value: m[1], Style: st})
				continue
			}
		}
		emitText(part)
	}
	if last < len(line) {
		emitText(line[last:])
	}
	return segs
}

// parseFile splits messages.txt into settings and message lines.
func parseFile(text string) (Settings, []string) {
	s := DefaultSettings()
	var lines []string
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "@") {
			k, v, _ := strings.Cut(line[1:], "=")
			applySetting(&s, strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v))
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		lines = []string{"(messages.txt is empty)"}
	}
	return s, lines
}

// applySetting applies one @key=value; it reports false for unknown keys or bad values.
func applySetting(s *Settings, k, v string) bool {
	switch k {
	case "step":
		if isDigits(v) {
			n, _ := strconv.Atoi(v)
			s.Step = max(1, min(n, 40))
			return true
		}
	case "gap":
		if isDigits(v) {
			s.Gap, _ = strconv.Atoi(v)
			return true
		}
	case "font":
		if v != "" {
			s.Font = v
			return true
		}
	case "size":
		if isDigits(v) {
			n, _ := strconv.Atoi(v)
			s.Size = max(8, min(n, led.Rows))
			return true
		}
	case "theme":
		s.Theme = strings.ToLower(v)
		return true
	case "location":
		s.Location = v
		return true
	case "colour", "color", "dim", "bg":
		if c, ok := parseColour(v); ok {
			switch k {
			case "dim":
				s.Dim = c
			case "bg":
				s.Bg = c
			default:
				s.Colour = c
			}
			return true
		}
	}
	return false
}

// fontKnown reports whether a font name resolves to something other than the fallback.
func fontKnown(name string) bool {
	if isFontAlias(name) {
		return true
	}
	if st, err := os.Stat(name); err == nil && !st.IsDir() {
		return true
	}
	k := normFont(name)
	if k == "" {
		return false
	}
	for stem := range led.InstalledFonts() {
		if strings.Contains(stem, k) {
			return true
		}
	}
	return false
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]`)

func normFont(s string) string { return nonAlnum.ReplaceAllString(strings.ToLower(s), "") }

// Lint returns human-readable warnings about messages.txt: unknown tags, unknown fonts, bad
// colours, unknown sprites and settings.
func Lint(text string) []string {
	var out []string
	for i, raw := range strings.Split(text, "\n") {
		n := i + 1
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "@") {
			k, v, hasEq := strings.Cut(line[1:], "=")
			k = strings.ToLower(strings.TrimSpace(k))
			v = strings.TrimSpace(v)
			s := DefaultSettings()
			if !hasEq {
				out = append(out, fmt.Sprintf("line %d: setting %q needs a value (@%s=...)", n, line, k))
			} else if !applySetting(&s, k, v) {
				out = append(out, fmt.Sprintf("line %d: unknown setting or bad value @%s=%s", n, k, v))
			} else if k == "theme" && v != "" && v != "green" && v != "amber" {
				out = append(out, fmt.Sprintf("line %d: theme %q is not green or amber; it is ignored", n, v))
			} else if k == "font" && !fontKnown(v) {
				out = append(out, fmt.Sprintf("line %d: font %q not found; sans is used", n, v))
			}
			continue
		}
		for _, part := range tagTokenRe.FindAllString(line, -1) {
			if strings.HasPrefix(part, ":") {
				name := strings.Trim(part, ":")
				if _, ok := spriteSpecs[name]; !ok {
					out = append(out, fmt.Sprintf("line %d: unknown sprite %s is shown as text (sprites: %s)", n, part, strings.Join(SpriteNames(), " ")))
				}
				continue
			}
			inner := part[1 : len(part)-1]
			if inner == "/" || inner == "" || inner == "reset" {
				continue
			}
			name, value, hasValue := strings.Cut(inner, "=")
			name = strings.ToLower(name)
			if _, ok := applyTag(Style{}, inner); !ok {
				switch {
				case hasValue && (name == "colour" || name == "color" || name == "c" || name == "bg"):
					out = append(out, fmt.Sprintf("line %d: bad colour %q in %s", n, value, part))
				case hasValue && (name == "size" || name == "s"):
					out = append(out, fmt.Sprintf("line %d: size must be a number in %s", n, part))
				default:
					out = append(out, fmt.Sprintf("line %d: unknown tag %s is shown as text", n, part))
				}
				continue
			}
			if hasValue && (name == "font" || name == "f") && !fontKnown(value) {
				out = append(out, fmt.Sprintf("line %d: font %q not found; sans is used", n, value))
			}
		}
	}
	return out
}

// Append adds one message line to the file, creating it with DefaultMessages if missing.
func Append(path, line string) error {
	line = strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(line))
	if line == "" {
		return fmt.Errorf("board: empty message")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(DefaultMessages), 0o644); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	// make sure the file ends with a newline before appending
	if st, err := f.Stat(); err == nil && st.Size() > 0 {
		b := make([]byte, 1)
		if rf, err := os.Open(path); err == nil {
			if _, err := rf.ReadAt(b, st.Size()-1); err == nil && b[0] != '\n' {
				line = "\n" + line
			}
			rf.Close()
		}
	}
	_, err = f.WriteString(line + "\n")
	return err
}
