package led

import (
	"encoding/json"
	"image/png"
	"os"
	"sort"
	"testing"
)

// TestTextParityWithPillow compares Text with board.py's text_cells. Run with
// TEXT_PARITY=<dir with cases.json from the Python harness>.
func TestTextParityWithPillow(t *testing.T) {
	dir := os.Getenv("TEXT_PARITY")
	if dir == "" {
		t.Skip()
	}
	var cases []struct {
		Font, Text, File string
		Size             int
	}
	b, _ := os.ReadFile(dir + "/cases.json")
	_ = json.Unmarshal(b, &cases)
	type res struct {
		name  string
		diff  int
		total int
	}
	var all []res
	exact := 0
	for _, c := range cases {
		f, err := os.Open(c.File)
		if err != nil {
			t.Fatal(err)
		}
		ref, _ := png.Decode(f)
		f.Close()
		m := Text(c.Text, c.Font, c.Size)
		rw, rh := ref.Bounds().Dx(), ref.Bounds().Dy()
		d, lit := 0, 0
		w := max(rw, m.W)
		for y := 0; y < max(rh, m.H); y++ {
			for x := 0; x < w; x++ {
				a := x < rw && y < rh && func() bool { r, _, _, _ := ref.At(x, y).RGBA(); return r > 0 }()
				g := m.At(x, y)
				if a {
					lit++
				}
				if a != g {
					d++
				}
			}
		}
		if rw != m.W {
			t.Logf("%s %d %q: width %d vs %d", c.Font, c.Size, c.Text, m.W, rw)
		}
		if d == 0 {
			exact++
		}
		all = append(all, res{c.Font + " " + itoa(c.Size) + " " + c.Text, d, lit})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].diff > all[j].diff })
	for _, r := range all[:min(15, len(all))] {
		t.Logf("worst: %-40s %d cells differ of %d lit", r.name, r.diff, r.total)
	}
	t.Logf("%d of %d masks identical", exact, len(cases))
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }
