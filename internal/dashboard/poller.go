package dashboard

import (
	"image"
	"strings"
	"sync"
	"time"
)

// poller fetches a value in the background: again every `every`, after a minute when the last
// try failed, and at once when key changes. A failed refetch keeps the last good value.
type poller[T any] struct {
	every time.Duration

	mu   sync.Mutex
	val  T
	ok   bool
	err  error
	key  string
	at   time.Time // last attempt
	got  time.Time // last success
	busy bool
}

func (p *poller[T]) get(key string, fetch func(key string) (T, error)) (val T, got time.Time, ok bool, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	due := key != p.key || time.Since(p.at) >= p.every || (!p.ok && time.Since(p.at) >= time.Minute)
	if !p.busy && due {
		if key != p.key {
			var zero T
			p.val, p.ok, p.err = zero, false, nil
		}
		p.busy, p.key, p.at = true, key, time.Now()
		go func() {
			v, err := fetch(key)
			p.mu.Lock()
			defer p.mu.Unlock()
			p.busy = false
			if key != p.key {
				return // asked for something else meanwhile
			}
			p.err = err
			if err == nil {
				p.val, p.ok, p.got = v, true, time.Now()
			}
		}()
	}
	return p.val, p.got, p.ok, p.err
}

// waitMessage is the frame shown while there is nothing to draw yet.
func waitMessage(c *frameCache, what string, err error) *image.RGBA {
	msg := "FETCHING " + what + "…"
	if err != nil {
		msg = strings.ToUpper(err.Error())
	}
	return c.get("msg"+msg, func(img *image.RGBA) {
		msg = fit(msg, 28, W-2*pad)
		text(img, (W-measure(msg, 28))/2, H/2-18, msg, 28, txt3)
	})
}
