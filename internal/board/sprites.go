package board

import "github.com/justynroberts/marquee/internal/led"

// Sprites are original 11x11 pixel art, scaled 4x to the 44-LED text height. Letters map to
// palette colours; "." is transparent.

var spriteColours = map[byte]led.RGB{
	'R': led.Palette["red"], 'O': led.Palette["orange"], 'Y': led.Palette["yellow"], 'G': led.Palette["green"],
	'C': led.Palette["cyan"], 'B': led.Palette["blue"], 'P': led.Palette["purple"], 'K': led.Palette["pink"],
	'W': led.Palette["white"], 'D': {R: 110, G: 110, B: 100},
}

type spriteSpec struct {
	fps    float64
	bob    bool
	frames [][]string
}

var spriteSpecs = map[string]spriteSpec{
	"heart": {fps: 3, frames: [][]string{
		{
			"..RR...RR..",
			".RRRR.RRRR.",
			"RRKRRRRRRRR",
			"RKRRRRRRRRR",
			"RRRRRRRRRRR",
			".RRRRRRRRR.",
			"..RRRRRRR..",
			"...RRRRR...",
			"....RRR....",
			".....R.....",
			"...........",
		},
		{
			"...........",
			"...R...R...",
			"..RRR.RRR..",
			"..RKRRRRR..",
			"..RRRRRRR..",
			"...RRRRR...",
			"....RRR....",
			".....R.....",
			"...........",
			"...........",
			"...........",
		},
	}},
	"fire": {fps: 8, frames: [][]string{
		{
			".....R.....",
			"....RR.....",
			"....ROR..R.",
			"...ROOR.RR.",
			"..ROOYORRO.",
			"..ROYYYOOR.",
			".ROYYYYYOR.",
			".ROYYWYYOR.",
			".ROYWWWYOR.",
			"..ROYWYOR..",
			"...RRRRR...",
		},
		{
			"......R....",
			".R...RR....",
			".RR..ROR...",
			"..RR.ROOR..",
			"..ROROYOR..",
			".ROOYYYOOR.",
			".ROYYYYYOR.",
			".ROYYWYYOR.",
			".ROYWWWYOR.",
			"..ROYWYOR..",
			"...RRRRR...",
		},
		{
			"....R......",
			"....RR..R..",
			"...RORR.RR.",
			"...ROOR.ROR",
			"..ROYOORROR",
			"..ROYYYOOR.",
			".ROYYYYYOR.",
			".ROYYWYYOR.",
			".ROYWWWYOR.",
			"..ROYWYOR..",
			"...RRRRR...",
		},
	}},
	"star": {fps: 5, frames: [][]string{
		{
			"...........",
			"...........",
			".....Y.....",
			".....Y.....",
			"...........",
			"..YY.W.YY..",
			"...........",
			".....Y.....",
			".....Y.....",
			"...........",
			"...........",
		},
		{
			"...........",
			".....Y.....",
			".....Y.....",
			"....YWY....",
			"...YWWWY...",
			".YYWWWWWYY.",
			"...YWWWY...",
			"....YWY....",
			".....Y.....",
			".....Y.....",
			"...........",
		},
		{
			".....Y.....",
			".Y...Y...Y.",
			"..Y..Y..Y..",
			"...YYWYY...",
			"...YWWWY...",
			"YYYWWWWWYYY",
			"...YWWWY...",
			"...YYWYY...",
			"..Y..Y..Y..",
			".Y...Y...Y.",
			".....Y.....",
		},
		{
			"...........",
			".....Y.....",
			".....Y.....",
			"....YWY....",
			"...YWWWY...",
			".YYWWWWWYY.",
			"...YWWWY...",
			"....YWY....",
			".....Y.....",
			".....Y.....",
			"...........",
		},
	}},
	"ufo": {fps: 4, bob: true, frames: [][]string{
		{
			"...........",
			"....CCC....",
			"...CWWWC...",
			"...CWWWC...",
			".DDDDDDDDD.",
			"DDDDDDDDDDD",
			"D.Y.D.G.D.R",
			".DDDDDDDDD.",
			"...........",
			"....Y.Y....",
			"...Y.Y.Y...",
		},
		{
			"...........",
			"....CCC....",
			"...CWWWC...",
			"...CWWWC...",
			".DDDDDDDDD.",
			"DDDDDDDDDDD",
			"D.G.D.R.D.Y",
			".DDDDDDDDD.",
			"...........",
			"...Y.Y.Y...",
			"....Y.Y....",
		},
	}},
	"wave": {fps: 6, frames: [][]string{
		{"...........", "...........", "...........", "...........", "C..........", "CC........C", "CCC......CC", "CCCC....CCC", "CCCCC..CCCC", "CCCCCCCCCCC", "BBBBBBBBBBB"},
		{"...........", "...........", "...........", "...........", "..........C", ".C.......CC", "CCC.....CCC", "CCCC...CCCC", "CCCCCC.CCCC", "CCCCCCCCCCC", "BBBBBBBBBBB"},
		{"...........", "...........", "...........", "...........", "...C.......", "..CCC......", ".CCCCC....C", "CCCCCCC..CC", "CCCCCCCCCCC", "CCCCCCCCCCC", "BBBBBBBBBBB"},
	}},
	"check": {fps: 2, frames: [][]string{
		{
			"...........",
			"..........G",
			".........GG",
			"........GG.",
			".......GG..",
			"G.....GG...",
			"GG...GG....",
			".GG.GG.....",
			"..GGG......",
			"...G.......",
			"...........",
		},
	}},
	"cross": {fps: 2, frames: [][]string{
		{
			"...........",
			".R.......R.",
			"..R.....R..",
			"...R...R...",
			"....R.R....",
			".....R.....",
			"....R.R....",
			"...R...R...",
			"..R.....R..",
			".R.......R.",
			"...........",
		},
		{
			"...........",
			".RR.....RR.",
			".RRR...RRR.",
			"..RRR.RRR..",
			"...RRRRR...",
			"....RRR....",
			"...RRRRR...",
			"..RRR.RRR..",
			".RRR...RRR.",
			".RR.....RR.",
			"...........",
		},
	}},
}

const (
	spriteScale = 4
	spriteW     = 11 * spriteScale // 44 LEDs
	spriteH     = 11 * spriteScale
	textRows    = 44 // sprite height and default text height in LEDs
)

// spriteFrame is one frame at LED resolution, indexed [y][x].
type spriteFrame struct {
	lit [spriteH][spriteW]bool
	col [spriteH][spriteW]led.RGB
}

var spriteFrames = map[string][]spriteFrame{}

func init() {
	for name, spec := range spriteSpecs {
		frames := make([]spriteFrame, len(spec.frames))
		for i, rows := range spec.frames {
			f := &frames[i]
			for y, line := range rows {
				for x := 0; x < len(line); x++ {
					c, ok := spriteColours[line[x]]
					if !ok {
						continue
					}
					for sy := range spriteScale {
						for sx := range spriteScale {
							f.lit[y*spriteScale+sy][x*spriteScale+sx] = true
							f.col[y*spriteScale+sy][x*spriteScale+sx] = c
						}
					}
				}
			}
		}
		spriteFrames[name] = frames
	}
}

// SpriteNames lists the :name: sprites.
func SpriteNames() []string {
	out := make([]string, 0, len(spriteSpecs))
	for _, n := range []string{"heart", "fire", "star", "ufo", "wave", "check", "cross"} {
		if _, ok := spriteSpecs[n]; ok {
			out = append(out, n)
		}
	}
	return out
}

func spriteFrameAt(name string, t float64) *spriteFrame {
	spec := spriteSpecs[name]
	frames := spriteFrames[name]
	i := int(t*spec.fps) % len(frames)
	if i < 0 {
		i += len(frames)
	}
	return &frames[i]
}
