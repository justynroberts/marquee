package dashboard

import (
	"image"
	"image/color"
	"time"
)

// WordClock spells the time to the nearest five minutes in a grid of letters, the words that
// say it lit and the rest barely there; dots underneath count the minutes in between.
type WordClock struct {
	cache frameCache
}

// a token is a word (key names it) or filler letters (key "").
type token struct{ text, key string }

// wcRows are the grid, 26 letters a row. Every phrase reads left to right, top to bottom.
var wcRows = [][]token{
	{{"IT", "it"}, {"L", ""}, {"IS", "is"}, {"A", "a"}, {"HALF", "half"}, {"TEN", "m10"}, {"QUARTER", "quarter"}, {"TWENTY", "twenty"}},
	{{"FIVE", "m5"}, {"K", ""}, {"MINUTES", "minutes"}, {"R", ""}, {"TO", "to"}, {"PAST", "past"}, {"Y", ""}, {"ONE", "h1"}, {"TWO", "h2"}},
	{{"THREE", "h3"}, {"S", ""}, {"FOUR", "h4"}, {"V", ""}, {"FIVE", "h5"}, {"M", ""}, {"SIX", "h6"}, {"Z", ""}, {"SEVEN", "h7"}, {"B", ""}},
	{{"EIGHT", "h8"}, {"NINE", "h9"}, {"Q", ""}, {"TEN", "h10"}, {"ELEVEN", "h11"}, {"J", ""}, {"TWELVE", "h12"}},
	{{"OCLOCK", "oclock"}, {"X", ""}, {"IN", "in"}, {"U", ""}, {"THE", "the"}, {"G", ""}, {"AFTERNOON", "afternoon"}, {"HBN", ""}},
	{{"MORNING", "morning"}, {"F", ""}, {"EVENING", "evening"}, {"D", ""}, {"AT", "at"}, {"C", ""}, {"NIGHT", "night"}, {"PE", ""}},
}

const wcCols = 26

// WordsFor is the set of words lit for t.
func WordsFor(t time.Time) map[string]bool {
	w := map[string]bool{"it": true, "is": true}
	m := t.Minute() / 5 * 5
	h := t.Hour()
	if m > 30 {
		h++
	}
	h12 := h % 12
	if h12 == 0 {
		h12 = 12
	}
	w["h"+itoa(h12)] = true
	switch m {
	case 0:
		w["oclock"] = true
	case 5, 55:
		w["m5"], w["minutes"] = true, true
	case 10, 50:
		w["m10"], w["minutes"] = true, true
	case 15, 45:
		w["a"], w["quarter"] = true, true
	case 20, 40:
		w["twenty"], w["minutes"] = true, true
	case 25, 35:
		w["twenty"], w["m5"], w["minutes"] = true, true, true
	case 30:
		w["half"] = true
	}
	if m != 0 {
		if m <= 30 {
			w["past"] = true
		} else {
			w["to"] = true
		}
	}
	switch hd := h % 24; {
	case hd >= 5 && hd < 12:
		w["in"], w["the"], w["morning"] = true, true, true
	case hd >= 12 && hd < 18:
		w["in"], w["the"], w["afternoon"] = true, true, true
	case hd >= 18 && hd < 22:
		w["in"], w["the"], w["evening"] = true, true, true
	default:
		w["at"], w["night"] = true, true
	}
	return w
}

// wordColour: the hour amber, morning/afternoon/evening/night cyan, the rest paper white.
func wordColour(key string) color.RGBA {
	switch key {
	case "in", "the", "morning", "afternoon", "evening", "at", "night":
		return cyan
	}
	if key[0] == 'h' && key != "half" {
		return amber
	}
	return txt
}

func itoa(n int) string {
	if n >= 10 {
		return string(rune('0'+n/10)) + string(rune('0'+n%10))
	}
	return string(rune('0' + n))
}

func NewWordClock() *WordClock { return &WordClock{} }

func (c *WordClock) Frame(t, dt float64, now time.Time) *image.RGBA {
	return c.cache.get(now.Format("15:04"), func(img *image.RGBA) { c.render(img, now) })
}

func (c *WordClock) render(img *image.RGBA, now time.Time) {
	lit := WordsFor(now)
	off := color.RGBA{48, 49, 44, 255}
	cw := float64(W-2*pad) / wcCols
	ch := 62
	px := 46
	y0 := 26
	for r, row := range wcRows {
		col := 0
		for _, tk := range row {
			c := off
			if tk.key != "" && lit[tk.key] {
				c = wordColour(tk.key)
			}
			for _, ch := range tk.text {
				s := string(ch)
				x := float64(pad) + (float64(col)+0.5)*cw - measure(s, px)/2
				text(img, x, y0+r*62, s, px, c)
				col++
			}
		}
	}
	// the minutes past the five
	n := now.Minute() % 5
	y := y0 + len(wcRows)*ch + 6
	for i := 0; i < 4; i++ {
		c := off
		if i < n {
			c = amber
		}
		x := W/2 + (i-2)*40 + 14
		rect(img, x, y, x+11, y+11, c)
	}
}
