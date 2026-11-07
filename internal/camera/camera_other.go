//go:build !darwin

package camera

import "errors"

const maxW, maxH = 320, 320

func start() error      { return errors.New("camera not supported on this platform") }
func stop()             {}
func setWant(f Feature) {}

func grab(dst, mask []byte) (w, h int, seq uint64, hasMask bool, hands []Hand) {
	return 0, 0, 0, false, nil
}
