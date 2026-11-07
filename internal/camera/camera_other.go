//go:build !darwin

package camera

import "errors"

const maxW, maxH = 320, 320

func start() error                           { return errors.New("camera not supported on this platform") }
func stop()                                  {}
func grab(dst []byte) (w, h int, seq uint64) { return 0, 0, 0 }
