//go:build !darwin

package location

import "errors"

func request() {}

func poll() (Fix, error, bool) { return Fix{}, errors.New("location: macOS only"), true }
