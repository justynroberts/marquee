package dashboard

import (
	"testing"
	"time"
)

func TestRender(t *testing.T) {
	d := New()
	defer d.Close()
	time.Sleep(600 * time.Millisecond)
	start := time.Now()
	img := d.Render()
	if el := time.Since(start); el > 80*time.Millisecond {
		t.Errorf("render took %s", el)
	}
	if img.Bounds().Dx() != W || img.Bounds().Dy() != H {
		t.Fatal("wrong size")
	}
	lit := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] > 150 {
			lit++
		}
	}
	if lit < 5000 {
		t.Errorf("only %d bright pixels", lit)
	}
}
