package scenes

import (
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/justynroberts/marquee/internal/camera"
	"github.com/justynroberts/marquee/internal/led"
)

// fakeCamera returns a moving gradient with a bright blob, a new frame every call. The blob is
// also the person mask, and its top is a pinching hand.
func fakeCamera() func(camera.Feature) camera.Frame {
	const w, h = 320, 240
	var seq uint64
	pix := make([]uint8, w*h*3)
	mask := make([]uint8, w*h)
	return func(want camera.Feature) camera.Frame {
		seq++
		cx := 160 + 100*math.Sin(float64(seq)*0.2)
		for y := range h {
			for x := range w {
				i := (y*w + x) * 3
				v := float64(x+y) / float64(w+h) * 120
				if d := math.Hypot(float64(x)-cx, float64(y)-120); d < 40 {
					v = 250
				}
				pix[i], pix[i+1], pix[i+2] = uint8(v), uint8(v*0.8), uint8(255-v)
				mask[y*w+x] = 0
				if v == 250 {
					mask[y*w+x] = 255
				}
			}
		}
		f := camera.Frame{W: w, H: h, Pix: pix, Seq: seq}
		if want&camera.Mask != 0 {
			f.Mask = mask
		}
		if want&camera.Hands != 0 {
			x := cx / w
			f.Hands = []camera.Hand{{
				Index: camera.Point{X: x, Y: 0.3, Conf: 0.9},
				Thumb: camera.Point{X: x + 0.01, Y: 0.32, Conf: 0.9},
				Wrist: camera.Point{X: x, Y: 0.7, Conf: 0.9},
			}}
		}
		return f
	}
}

func TestCameraProblemScreens(t *testing.T) {
	for _, f := range []camera.Frame{
		{Err: camera.ErrDenied, Denied: true},
		{Err: camera.ErrPending},
		{Err: errString("no camera")},
		{}, // starting
	} {
		CameraSource = func(camera.Feature) camera.Frame { return f }
		for _, info := range List() {
			if info.Group != "camera" {
				continue
			}
			g := led.NewGrid()
			info.New().Draw(g, 0, 0.1)
			if litCount(g) == 0 {
				t.Errorf("%s: no message for %+v", info.Name, f)
			}
		}
	}
}

func TestMotionStillIsDark(t *testing.T) {
	still := fakeCamera()(0)
	n := still.Seq
	CameraSource = func(camera.Feature) camera.Frame { n++; f := still; f.Seq = n; return f }
	m := &motion{}
	var g *led.Grid
	for i := range 24 {
		g = led.NewGrid()
		m.Draw(g, float64(i)/12, 1.0/12)
	}
	if c := litCount(g); c != 0 {
		t.Fatalf("still picture lit %d LEDs", c)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

// TestCameraReal opens the real camera (CAMERA_REAL=DIR) and writes a PNG per camera scene.
func TestCameraReal(t *testing.T) {
	dir := os.Getenv("CAMERA_REAL")
	if dir == "" {
		t.Skip("CAMERA_REAL=DIR not set")
	}
	CameraSource = camera.LatestWith
	defer camera.Stop()
	deadline := time.Now().Add(10 * time.Second)
	for f := camera.Latest(); f.W == 0; f = camera.Latest() {
		if f.Err != nil && f.Err != camera.ErrPending {
			t.Fatal(f.Err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("no frame in 10 s (err %v)", f.Err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	for _, name := range []string{"camera", "popart", "motion", "slitscan", "snowfall", "bubbles", "airpaint"} {
		info, _ := Get(name)
		sc := info.New()
		var g *led.Grid
		for i := range 36 {
			g = led.NewGrid()
			sc.Draw(g, float64(i)/12, 1.0/12)
			time.Sleep(time.Second / 12)
		}
		out, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(out, g.Image(led.RGB{R: 12, G: 13, B: 11})); err != nil {
			t.Fatal(err)
		}
		out.Close()
		t.Logf("%s: %d LEDs lit", name, litCount(g))
	}
}

// block is a still frame whose person mask is a rectangle (frame pixels x0..x1, y0..y1).
func block(x0, y0, x1, y1 int, hands []camera.Hand) func(camera.Feature) camera.Frame {
	const w, h = 320, 240
	pix := make([]uint8, w*h*3)
	mask := make([]uint8, w*h)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			mask[y*w+x] = 255
		}
	}
	var seq uint64
	return func(camera.Feature) camera.Frame {
		seq++
		return camera.Frame{W: w, H: h, Pix: pix, Seq: seq, Mask: mask, Hands: hands}
	}
}

func TestSnowSettlesOnBody(t *testing.T) {
	CameraSource = block(100, 120, 220, 240, nil) // lower half, centre: shoulders
	s := &snowfall{}
	for i := range 400 {
		s.Draw(led.NewGrid(), float64(i)/12, 1.0/12)
	}
	// The row just above the body's top edge (grid y = 33) should carry snow over the body.
	on := 0
	for x := range cols {
		if s.body[33][x] && s.snow[32][x] {
			on++
		}
	}
	if on < 10 {
		t.Fatalf("only %d snow cells resting on the body", on)
	}
}

func TestBubblesPop(t *testing.T) {
	CameraSource = block(0, 0, 320, 240, nil) // the person fills the frame
	b := &bubbles{}
	for i := range 120 {
		b.Draw(led.NewGrid(), float64(i)/12, 1.0/12)
	}
	if b.pops == 0 {
		t.Fatal("no bubble popped")
	}
}

func TestAirpaintPinchDraws(t *testing.T) {
	hand := func(gap float64) []camera.Hand {
		return []camera.Hand{{
			Index: camera.Point{X: 0.5, Y: 0.5, Conf: 0.9},
			Thumb: camera.Point{X: 0.5 + gap, Y: 0.5, Conf: 0.9},
			Wrist: camera.Point{X: 0.5, Y: 0.9, Conf: 0.9},
		}}
	}
	for _, c := range []struct {
		gap  float64
		want bool
	}{{0.01, true}, {0.2, false}} {
		CameraSource = block(0, 0, 0, 0, hand(c.gap))
		a := &airpaint{}
		for i := range 6 {
			a.Draw(led.NewGrid(), float64(i)/12, 1.0/12)
		}
		if a.drawn != c.want {
			t.Errorf("gap %v: drawn=%v want %v", c.gap, a.drawn, c.want)
		}
	}
}
