package led

import (
	"fmt"
	"image"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/justynroberts/marquee/internal/ft"
)

// Fonts. Shortcut names map to system fonts where present (macOS paths first, then common Linux
// ones), and fall back to the embedded Go fonts so a bare binary always renders text.

type fontRef struct {
	paths []string
	index int // face index inside a .ttc
}

const (
	macSys = "/System/Library/Fonts"
	macSup = "/System/Library/Fonts/Supplemental"
)

var aliases = map[string]fontRef{
	"sans":       {[]string{macSys + "/Helvetica.ttc", "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"}, 1},
	"serif":      {[]string{macSup + "/Georgia Bold.ttf", "/usr/share/fonts/truetype/dejavu/DejaVuSerif-Bold.ttf"}, 0},
	"mono":       {[]string{macSys + "/Menlo.ttc", "/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf"}, 1},
	"script":     {[]string{macSup + "/SnellRoundhand.ttc"}, 1},
	"condensed":  {[]string{macSup + "/Impact.ttf"}, 0},
	"rounded":    {[]string{macSup + "/Arial Rounded Bold.ttf"}, 0},
	"typewriter": {[]string{macSup + "/AmericanTypewriter.ttc"}, 2},
	"chalk":      {[]string{macSup + "/Chalkduster.ttf"}, 0},
	"stencil":    {[]string{macSup + "/Copperplate.ttc"}, 2},
	"didot":      {[]string{macSup + "/Didot.ttc"}, 2},
	"futura":     {[]string{macSup + "/Futura.ttc"}, 2},
	"marker":     {[]string{macSys + "/MarkerFelt.ttc"}, 1},
	"hand":       {[]string{macSup + "/Bradley Hand Bold.ttf"}, 0},
	"poster":     {[]string{macSup + "/Phosphate.ttc"}, 1},
	"cjk":        {[]string{"/Library/Fonts/Arial Unicode.ttf", macSup + "/Arial Unicode.ttf"}, 0},
	"pixel":      {nil, 0}, // built-in bitmap font scaled up
}

// FontNames lists the shortcut names.
func FontNames() []string {
	out := make([]string, 0, len(aliases))
	for k := range aliases {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// fontDirs are scanned non-recursively, in this order, like board.py's FONT_DIRS; the Linux
// folders (scanned recursively) come last.
var fontDirs = []string{macSys, macSup, "/Library/Fonts", filepath.Join(os.Getenv("HOME"), "Library/Fonts")}
var linuxFontDirs = []string{"/usr/share/fonts", "/usr/local/share/fonts", filepath.Join(os.Getenv("HOME"), ".fonts")}

var (
	idxOnce   sync.Once
	fontIndex map[string]string
	fontOrder []string // stems in discovery order (Python dict order)
	nonAlnum  = regexp.MustCompile(`[^a-z0-9]`)
)

func norm(s string) string { return nonAlnum.ReplaceAllString(strings.ToLower(s), "") }

func isFontFile(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	return ext == ".ttf" || ext == ".ttc" || ext == ".otf"
}

// InstalledFonts maps normalised file stems to paths.
func InstalledFonts() map[string]string {
	idxOnce.Do(func() {
		fontIndex = map[string]string{}
		add := func(p string) {
			stem := norm(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)))
			if _, ok := fontIndex[stem]; !ok {
				fontIndex[stem] = p
				fontOrder = append(fontOrder, stem)
			}
		}
		for _, d := range fontDirs {
			entries, _ := os.ReadDir(d)
			for _, e := range entries {
				if !e.IsDir() && isFontFile(e.Name()) {
					add(filepath.Join(d, e.Name()))
				}
			}
		}
		for _, d := range linuxFontDirs {
			_ = filepath.WalkDir(d, func(p string, e os.DirEntry, err error) error {
				if err == nil && !e.IsDir() && isFontFile(p) {
					add(p)
				}
				return nil
			})
		}
	})
	return fontIndex
}

// ResolveFont is board.py's resolve_font: a shortcut, a file path, or an installed font name
// (exact, +bold, +regular, then fuzzy preferring bold and shorter names) -> (path, ttc index).
// ok is false when nothing matched and the sans fallback was returned.
func ResolveFont(name string) (path string, index int, ok bool) {
	key := strings.TrimSpace(name)
	if ref, found := aliases[strings.ToLower(key)]; found {
		for _, p := range ref.paths {
			if _, err := os.Stat(p); err == nil {
				return p, ref.index, true
			}
		}
		if len(ref.paths) == 0 {
			return "", 0, true // pixel
		}
		return "", 0, false
	}
	if strings.HasPrefix(key, "~/") {
		key = filepath.Join(os.Getenv("HOME"), key[2:])
	}
	// A path is only followed to an actual font file, so board text cannot probe the disk.
	switch strings.ToLower(filepath.Ext(key)) {
	case ".ttf", ".otf", ".ttc", ".otc", ".dfont":
		if st, err := os.Stat(key); err == nil && !st.IsDir() {
			return key, 0, true
		}
	}
	fonts := InstalledFonts()
	n := norm(key)
	for _, want := range []string{n, n + "bold", n + "regular"} {
		if p, found := fonts[want]; found {
			return p, 0, true
		}
	}
	var cands []string
	for _, stem := range fontOrder {
		if strings.HasPrefix(stem, n) || strings.Contains(stem, n) {
			cands = append(cands, stem)
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		bi, bj := strings.Contains(cands[i], "bold"), strings.Contains(cands[j], "bold")
		if bi != bj {
			return bi
		}
		return len(cands[i]) < len(cands[j])
	})
	if len(cands) > 0 {
		return fonts[cands[0]], 0, true
	}
	return "", 0, false
}

type faceKey struct {
	name string
	px   int
}

var (
	faceMu    sync.Mutex
	faceCache = map[faceKey]font.Face{}
	parsed    = map[string]*opentype.Font{}
)

func loadFont(path string, index int) *opentype.Font {
	key := path + "#" + string(rune('0'+index))
	if f, ok := parsed[key]; ok {
		return f
	}
	data, err := os.ReadFile(path)
	if err != nil {
		parsed[key] = nil
		return nil
	}
	var f *opentype.Font
	if strings.HasSuffix(strings.ToLower(path), ".ttc") {
		if c, err := opentype.ParseCollection(data); err == nil {
			if index >= c.NumFonts() {
				index = 0
			}
			f, _ = c.Font(index)
		}
	} else {
		f, _ = opentype.Parse(data)
	}
	parsed[key] = f
	return f
}

var (
	goBold, _     = opentype.Parse(gobold.TTF)
	goMonoBold, _ = opentype.Parse(gomonobold.TTF)
)

// resolve returns a parsed font for a shortcut, an installed font name, or a path.
func resolve(name string) *opentype.Font {
	n := strings.ToLower(strings.TrimSpace(name))
	if ref, ok := aliases[n]; ok {
		for _, p := range ref.paths {
			if f := loadFont(p, ref.index); f != nil {
				return f
			}
		}
		if n == "mono" {
			return goMonoBold
		}
		return goBold
	}
	if st, err := os.Stat(name); err == nil && !st.IsDir() {
		if f := loadFont(name, 0); f != nil {
			return f
		}
	}
	fonts := InstalledFonts()
	k := norm(name)
	for _, want := range []string{k, k + "bold", k + "regular"} {
		if p, ok := fonts[want]; ok {
			if f := loadFont(p, 0); f != nil {
				return f
			}
		}
	}
	var cands []string
	for stem := range fonts {
		if strings.HasPrefix(stem, k) || strings.Contains(stem, k) {
			cands = append(cands, stem)
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		bi, bj := strings.Contains(cands[i], "bold"), strings.Contains(cands[j], "bold")
		if bi != bj {
			return bi
		}
		return len(cands[i]) < len(cands[j])
	})
	for _, c := range cands {
		if f := loadFont(fonts[c], 0); f != nil {
			return f
		}
	}
	return goBold
}

// Face returns a cached face for a font name at a pixel size.
func Face(name string, px int) font.Face {
	faceMu.Lock()
	defer faceMu.Unlock()
	k := faceKey{name, px}
	if f, ok := faceCache[k]; ok {
		return f
	}
	f := resolve(name)
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: float64(px), DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		face, _ = opentype.NewFace(goBold, &opentype.FaceOptions{Size: float64(px), DPI: 72})
	}
	faceCache[k] = face
	return face
}

// Text is board.py's text_cells: text drawn at LED resolution with FreeType exactly as Pillow
// does, as a mask Rows tall with the cap height of "H0" centred. size is clamped to 8..Rows; the
// font is int(size*1.32) px. Font "pixel" is Pillow's built-in bitmap font, scaled up.
func Text(s, fontName string, size int) *Mask {
	size = max(8, min(size, Rows))
	if strings.ToLower(strings.TrimSpace(fontName)) == "pixel" {
		return pixelText(s, size)
	}
	f := truetype(fontName, int(float64(size)*1.32))
	if f == nil {
		return textGo(s, fontName, size)
	}
	capBox := f.BBox("H0")
	top := floorDiv(Rows-(capBox.Max.Y-capBox.Min.Y), 2) - capBox.Min.Y
	w := max(1, int(math.Ceil(f.Length(s)))+2)
	img := image.NewAlpha(image.Rect(0, 0, w, Rows))
	f.Draw(img, 0, top, s)
	mask := NewMask(w, Rows)
	for i, v := range img.Pix {
		mask.Bits[i] = v > 110
	}
	return mask
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

var (
	ttMu    sync.Mutex
	ttCache = map[string]*ft.Face{}
)

// truetype is board.py's truetype(): the resolved font at px, falling back to index 0, then to
// the sans shortcut; nil only when no font file exists at all (then the embedded Go font is used).
func truetype(name string, px int) *ft.Face {
	k := fmt.Sprintf("%s@%d", strings.ToLower(name), px)
	ttMu.Lock()
	defer ttMu.Unlock()
	if f, ok := ttCache[k]; ok {
		return f
	}
	path, index, ok := ResolveFont(name)
	var f *ft.Face
	try := func(p string, i int) bool {
		if p == "" {
			return false
		}
		face, err := ft.Open(p, i, px)
		if err != nil {
			return false
		}
		f = face
		return true
	}
	if !(ok && (try(path, index) || try(path, 0))) {
		for _, p := range aliases["sans"].paths {
			if try(p, aliases["sans"].index) || try(p, 0) {
				break
			}
		}
	}
	if f == nil {
		if p := embeddedFontPath(strings.ToLower(name) == "mono"); p != "" {
			try(p, 0)
		}
	}
	ttCache[k] = f
	return f
}

var (
	embedOnce  sync.Once
	embedPaths [2]string
)

// embeddedFontPath writes the embedded Go fonts to the cache dir once, for machines with no fonts.
func embeddedFontPath(mono bool) string {
	embedOnce.Do(func() {
		dir, err := os.UserCacheDir()
		if err != nil {
			dir = os.TempDir()
		}
		dir = filepath.Join(dir, "marquee-fonts")
		_ = os.MkdirAll(dir, 0o755)
		for i, f := range []struct {
			name string
			data []byte
		}{{"gobold.ttf", gobold.TTF}, {"gomonobold.ttf", gomonobold.TTF}} {
			p := filepath.Join(dir, f.name)
			if os.WriteFile(p, f.data, 0o644) == nil {
				embedPaths[i] = p
			}
		}
	})
	if mono {
		return embedPaths[1]
	}
	return embedPaths[0]
}

// textGo is the pure-Go fallback used only if FreeType cannot open any font.
func textGo(s, fontName string, size int) *Mask {
	face := Face(fontName, int(float64(size)*1.32))
	b, _ := font.BoundString(face, "H0")
	capH := (b.Max.Y - b.Min.Y).Ceil()
	baseline := (Rows-capH)/2 - b.Min.Y.Floor()
	w := font.MeasureString(face, s).Ceil() + 2
	img := image.NewAlpha(image.Rect(0, 0, w, Rows))
	d := &font.Drawer{Dst: img, Src: image.Opaque, Face: face, Dot: fixed.P(0, baseline)}
	d.DrawString(s)
	mask := NewMask(w, Rows)
	for i, v := range img.Pix {
		mask.Bits[i] = v > 110
	}
	return mask
}

// TextAt renders text with its ink starting at row top (for small labels).
func TextAt(s, fontName string, size, top int) *Mask {
	m := Text(s, fontName, size)
	t, _ := m.InkRows()
	if t < 0 {
		return m
	}
	shift := top - t
	out := NewMask(m.W, m.H)
	for y := 0; y < m.H; y++ {
		ny := y + shift
		if ny < 0 || ny >= m.H {
			continue
		}
		copy(out.Bits[ny*m.W:(ny+1)*m.W], m.Bits[y*m.W:(y+1)*m.W])
	}
	return out
}

// pixelText is text_cells' "pixel" branch: Pillow's bitmap font (glyphs pasted over each other,
// exactly as Pillow's font_getmask does), scaled up with nearest neighbour.
func pixelText(s string, size int) *Mask {
	// Pillow encodes ImageFont text as Latin-1; other characters have no glyph.
	var codes []int
	w := 0
	for _, r := range s {
		c := int(r)
		if c > 255 {
			c = '?'
		}
		codes = append(codes, c)
		w += int(pilMetrics[c][0])
	}
	w = max(1, w)
	src := make([]bool, w*pilYSize)
	x := 0
	for _, c := range codes {
		g := pilMetrics[c]
		dx, dx0, dy0 := int(g[0]), int(g[2]), int(g[3])
		sx0, sy0, sx1, sy1 := int(g[6]), int(g[7]), int(g[8]), int(g[9])
		for r := 0; r < sy1-sy0; r++ {
			for q := 0; q < sx1-sx0; q++ {
				X, Y := x+dx0+q, pilBaseline+dy0+r
				if X >= 0 && X < w && Y >= 0 && Y < pilYSize {
					src[Y*w+X] = pilBitmap[sy0+r][sx0+q] == '1'
				}
			}
		}
		x += dx
	}
	// Cap height from the ink of "H" (rows 3..9 of the bitmap font).
	capTop, capBottom := pilCapBox()
	capH := max(1, capBottom-capTop)
	scale := max(1, int(math.RoundToEven(float64(size)*0.72/float64(capH))))
	mask := NewMask(w*scale, Rows)
	topRow := floorDiv(Rows-capH*scale, 2) - capTop*scale
	for y := 0; y < pilYSize*scale; y++ {
		my := topRow + y
		if my < 0 || my >= Rows {
			continue
		}
		for xx := 0; xx < w*scale; xx++ {
			mask.Bits[my*mask.W+xx] = src[(y/scale)*w+xx/scale]
		}
	}
	return mask
}

var (
	capOnce          sync.Once
	pilCapT, pilCapB int
)

// pilCapBox measures the ink box of "H" in the bitmap font, as text_cells' probe does.
func pilCapBox() (int, int) {
	capOnce.Do(func() {
		g := pilMetrics['H']
		dy0, sx0, sy0, sx1, sy1 := int(g[3]), int(g[6]), int(g[7]), int(g[8]), int(g[9])
		pilCapT, pilCapB = 1<<30, -1
		for r := 0; r < sy1-sy0; r++ {
			for q := 0; q < sx1-sx0; q++ {
				if pilBitmap[sy0+r][sx0+q] == '1' {
					y := pilBaseline + dy0 + r
					pilCapT, pilCapB = min(pilCapT, y), max(pilCapB, y+1)
				}
			}
		}
	})
	return pilCapT, pilCapB
}

// MeasureText returns the LED width a string would take.
func MeasureText(s, fontName string, size int) int { return Text(s, fontName, size).W }

// keep draw imported for callers that compose images
var _ = draw.Src
