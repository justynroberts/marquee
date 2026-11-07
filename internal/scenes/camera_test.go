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

// fakeCamera returns a moving gradient with a bright blob, a new frame every call.
func fakeCamera() func() camera.Frame {
	const w, h = 320, 240
	var seq uint64
	pix := make([]uint8, w*h*3)
	return func() camera.Frame {
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
			}
		}
		return camera.Frame{W: w, H: h, Pix: pix, Seq: seq}
	}
}

func TestCameraProblemScreens(t *testing.T) {
	for _, f := range []camera.Frame{
		{Err: camera.ErrDenied, Denied: true},
		{Err: camera.ErrPending},
		{Err: errString("no camera")},
		{}, // starting
	} {
		CameraSource = func() camera.Frame { return f }
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
	still := fakeCamera()()
	n := still.Seq
	CameraSource = func() camera.Frame { n++; f := still; f.Seq = n; return f }
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
	CameraSource = camera.Latest
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
	for _, name := range []string{"camera", "popart", "motion", "slitscan"} {
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
