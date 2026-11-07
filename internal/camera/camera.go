// Package camera captures the default camera for the camera scenes, as small mirrored RGB
// frames (at most 320 px wide). Like the microphone, the camera opens on first use and is
// released 10 seconds after the last scene stops asking, so macOS stops showing it in use.
//
// Opening the camera can take a second, so it happens on its own goroutine: Latest never
// blocks the render loop.
package camera

import (
	"errors"
	"log/slog"
	"sync"
	"time"
)

// ErrDenied and ErrPending are reported in Frame.Err when macOS has refused camera access, or
// is still showing the permission prompt.
var (
	ErrDenied  = errors.New("camera access denied")
	ErrPending = errors.New("waiting for camera permission")
)

// Frame is one camera image. Pix is W*H RGB triples, row by row, mirrored horizontally. It is
// shared between calls: treat it as read-only. W == 0 means no frame has arrived yet.
type Frame struct {
	W, H   int
	Pix    []uint8
	Seq    uint64 // increases with every new camera frame
	Err    error  // capture could not start (nil while starting or running)
	Denied bool   // Err is ErrDenied

	// Mask is the person segmentation, W*H values 0..255 (255 = person), when Mask was asked
	// for and Vision produced one; nil otherwise.
	Mask []uint8
	// Hands are the hands Vision found, when Hands was asked for (at most two).
	Hands []Hand
}

// Feature selects the Vision analyses LatestWith runs on each camera frame. They cost CPU, so
// they run only while a scene keeps asking for them.
type Feature int

const (
	Mask  Feature = 1 // person segmentation (Frame.Mask)
	Hands Feature = 2 // hand pose (Frame.Hands)
)

// Point is a position in 0..1 frame coordinates (top-left origin, mirrored like the frame)
// with Vision's confidence; Conf 0 means the point was not seen.
type Point struct{ X, Y, Conf float64 }

// Hand is one detected hand.
type Hand struct{ Index, Thumb, Wrist Point }

// MaskAt reports whether the frame pixel (x, y) is part of a person.
func (f Frame) MaskAt(x, y int) bool {
	if f.Mask == nil || x < 0 || y < 0 || x >= f.W || y >= f.H {
		return false
	}
	return f.Mask[y*f.W+x] >= 128
}

// At returns the pixel at (x, y), clamped to the frame.
func (f Frame) At(x, y int) (r, g, b uint8) {
	x = min(max(x, 0), f.W-1)
	y = min(max(y, 0), f.H-1)
	i := (y*f.W + x) * 3
	return f.Pix[i], f.Pix[i+1], f.Pix[i+2]
}

const (
	idleRelease = 10 * time.Second
	retryEvery  = 3 * time.Second
)

var (
	mu       sync.Mutex
	running  bool
	starting bool
	lastErr  error
	lastTry  time.Time
	lastUse  time.Time
	idleOnce sync.Once
	buf      = make([]byte, maxW*maxH*3)
	maskBuf  = make([]byte, maxW*maxH)
	wanted   Feature
	cur      Frame
)

// Latest returns the newest frame, opening the camera if it is not open. If opening fails it
// is retried every few seconds (permission may be granted, or a camera plugged in, meanwhile).
func Latest() Frame { return LatestWith(0) }

// LatestWith is Latest, also asking for the given Vision analyses on coming frames (and no
// others). The first frames after a change may not carry them yet.
func LatestWith(want Feature) Frame {
	mu.Lock()
	defer mu.Unlock()
	lastUse = time.Now()
	idleOnce.Do(func() { go releaseWhenIdle() })
	if !running && !starting && time.Since(lastTry) > retryEvery {
		starting, lastTry = true, time.Now()
		go open()
	}
	if want != wanted {
		wanted = want
		setWant(want)
	}
	if running {
		if w, h, seq, hasMask, hands := grab(buf, maskBuf); seq != 0 && seq != cur.Seq {
			cur = Frame{W: w, H: h, Pix: buf[:w*h*3], Seq: seq, Hands: hands}
			if hasMask {
				cur.Mask = maskBuf[:w*h]
			}
		}
	}
	f := cur
	if !running {
		f = Frame{}
	}
	f.Err, f.Denied = lastErr, errors.Is(lastErr, ErrDenied)
	return f
}

func open() {
	err := start()
	mu.Lock()
	defer mu.Unlock()
	starting = false
	if err != nil {
		if lastErr == nil || lastErr.Error() != err.Error() {
			slog.Warn("camera capture failed", "err", err)
		}
		lastErr = err
		return
	}
	slog.Info("camera capture started")
	running, lastErr = true, nil
	if time.Since(lastUse) > idleRelease { // the scene stopped while the camera was opening
		running = false
		stop()
		wanted = 0
	}
}

func releaseWhenIdle() {
	for range time.Tick(2 * time.Second) {
		mu.Lock()
		if running && time.Since(lastUse) > idleRelease {
			running, cur = false, Frame{}
			stop()
			wanted = 0
			slog.Info("camera released (no camera scene playing)")
		}
		mu.Unlock()
	}
}

// Stop releases the camera if it is open.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if running {
		running, cur = false, Frame{}
		stop()
		wanted = 0
	}
}
