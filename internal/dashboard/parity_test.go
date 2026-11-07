package dashboard

import (
	"image"
	"image/png"
	"math"
	"os"
	"testing"
	"time"
)

// TestParityWithPython renders the same fixed sample as the Python dashboard and compares.
// Run with DASH_PARITY=<dir containing py.png>.
func TestParityWithPython(t *testing.T) {
	dir := os.Getenv("DASH_PARITY")
	if dir == "" {
		t.Skip()
	}
	var s sample
	for i := 0; i < 60; i++ {
		s.cpuH = append(s.cpuH, 30+20*math.Sin(float64(i)/5))
		s.rxH = append(s.rxH, 50000+40000*math.Sin(float64(i)/3))
		s.txH = append(s.txH, 20000+15000*math.Cos(float64(i)/4))
	}
	s.cpu, s.cores, s.load = 32.4, []float64{40, 38, 30, 22, 45, 30, 18, 10}, [3]float64{3.27, 3.97, 3.58}
	s.memPct, s.memUsed, s.memT, s.swap = 66.2, 10.5*(1<<30), 16*(1<<30), 2.2*(1<<30)
	s.rx, s.tx = 233*1024, 183*1024
	s.batt = &battery{percent: 87}
	s.uptime = (3*24+6)*time.Hour + 23*time.Minute
	s.procs, s.top = 847, "zoom.us"
	d := &Dashboard{host: "justyns-macbook-pro", osVer: "macOS 27.0.1"}
	img := d.renderAt(s, time.Date(2026, 11, 6, 18, 14, 2, 0, time.Local))
	f, _ := os.Create(dir + "/go.png")
	_ = png.Encode(f, img)
	f.Close()
	rf, err := os.Open(dir + "/py.png")
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := png.Decode(rf)
	rf.Close()
	var sum float64
	diffPx := 0
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			r, g, b, _ := ref.At(x, y).RGBA()
			c := img.RGBAAt(x, y)
			dd := math.Abs(float64(c.R)-float64(r>>8)) + math.Abs(float64(c.G)-float64(g>>8)) + math.Abs(float64(c.B)-float64(b>>8))
			sum += dd
			if dd > 60 {
				diffPx++
			}
		}
	}
	t.Logf("mean abs diff %.3f, strongly different pixels %d (%.2f%%)", sum/float64(W*H*3), diffPx, 100*float64(diffPx)/float64(W*H))
	_ = image.Rect
}
