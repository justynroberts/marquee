package panel

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"sync"
	"time"

	"github.com/justynroberts/marquee/internal/jpeg444"
	"github.com/justynroberts/marquee/internal/usb"
)

// Known panels. Width comes from the handshake; height is per model.
type Model struct {
	VID, PID      uint16
	Name          string
	Width, Height int
	// Rotate180: the panel's scan order is upside down relative to how it is normally mounted,
	// so frames are flipped before sending (what Thermalright's own software does).
	Rotate180 bool
}

var Supported = []Model{
	{VID: 0x0416, PID: 0x5408, Name: "Thermalright Trofeo Vision 9.16", Width: 1920, Height: 462, Rotate180: true},
}

// Panel is an open, handshaken display.
type Panel struct {
	Model Model
	Info  Info
	dev   *usb.Device
	mu    sync.Mutex
	// Quality is the JPEG quality used by SendImage (1-100).
	Quality int
	// Flip inverts the model's default orientation, for panels mounted the other way up.
	Flip bool
}

// Open finds the first supported panel, claims it and runs the handshake.
func Open() (*Panel, error) {
	var lastErr error = usb.ErrNotFound
	for _, m := range Supported {
		dev, err := usb.Open(m.VID, m.PID, 0)
		if err != nil {
			lastErr = err
			continue
		}
		p := &Panel{Model: m, dev: dev, Quality: 88}
		if err := p.handshake(); err != nil {
			// The panel can be left mid-conversation by a previous program (an unread status
			// reply, a half-sent frame). Drain, clear stalls, reset, and try once more.
			p.recover()
			if err2 := p.handshake(); err2 != nil {
				dev.Close()
				return nil, fmt.Errorf("%v (after USB reset: %v)", err, err2)
			}
		}
		if p.Info.Width > 0 && p.Info.Width != m.Width {
			p.Model.Width = p.Info.Width
		}
		return p, nil
	}
	return nil, lastErr
}

func (p *Panel) recover() {
	for i := 0; i < 8; i++ {
		if _, err := p.dev.Read(512, 50); err != nil {
			break
		}
	}
	p.dev.ClearHalt()
	_ = p.dev.Reset()
	time.Sleep(300 * time.Millisecond)
	for i := 0; i < 8; i++ {
		if _, err := p.dev.Read(512, 50); err != nil {
			break
		}
	}
}

func (p *Panel) handshake() error {
	if _, err := p.dev.Write(HandshakeRequest(), 2000); err != nil {
		return fmt.Errorf("handshake write: %w", err)
	}
	reply, err := p.dev.Read(512, 2000)
	if err != nil {
		return fmt.Errorf("handshake read: %w", err)
	}
	info, err := ParseHandshake(reply)
	if err != nil {
		return err
	}
	p.Info = info
	return nil
}

// SendJPEG pushes an already-encoded baseline JPEG of the panel's size.
func (p *Panel) SendJPEG(data []byte) error {
	writes, err := Packets(data)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, w := range writes {
		if _, err := p.dev.Write(w, 2000); err != nil {
			return err
		}
	}
	// Every frame is answered with a 512-byte status. Read it, or the panel stops accepting
	// writes (it waits for the host to collect the reply).
	reply, err := p.dev.Read(512, 3000)
	if err != nil {
		return fmt.Errorf("frame status: %w", err)
	}
	if len(reply) < 2 || reply[0] != 0x03 || reply[1] != 0xff {
		return fmt.Errorf("unexpected frame status (% x)", head(reply, 16))
	}
	return nil
}

// SendImage scales nothing: the image should already be Width x Height.
func (p *Panel) SendImage(img image.Image) error {
	if p.Model.Rotate180 != p.Flip {
		img = Rotate180(img)
	}
	data, err := EncodeJPEG(img, p.Quality)
	if err != nil {
		return err
	}
	return p.SendJPEG(data)
}

// Rotate180 returns a copy of img turned upside down.
func Rotate180(img image.Image) *image.RGBA {
	b := img.Bounds()
	src, ok := img.(*image.RGBA)
	if !ok {
		src = image.NewRGBA(b)
		draw.Draw(src, b, img, b.Min, draw.Src)
	}
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	n := len(src.Pix) / 4
	for i := 0; i < n; i++ {
		j := n - 1 - i
		copy(dst.Pix[j*4:j*4+4], src.Pix[i*4:i*4+4])
	}
	return dst
}

// EncodeJPEG encodes as baseline 4:4:4 JPEG, the only form the panel accepts.
func EncodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if _, ok := img.(*image.RGBA); !ok {
		b := img.Bounds()
		rgba := image.NewRGBA(b)
		draw.Draw(rgba, b, img, b.Min, draw.Src)
		img = rgba
	}
	if err := jpeg444.Encode(&buf, img, &jpeg444.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Close releases the panel.
func (p *Panel) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dev.Close()
}

// Size returns the drawable size.
func (p *Panel) Size() (int, int) { return p.Model.Width, p.Model.Height }

// Sleep briefly between frames when a caller wants a fixed rate.
func Pace(start time.Time, interval time.Duration) {
	if d := interval - time.Since(start); d > 0 {
		time.Sleep(d)
	}
}
