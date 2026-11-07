package main

import (
	"errors"
	"flag"
	"image/png"
	"os"
	"strings"

	"github.com/justynroberts/marquee/internal/engine"
)

// render draws a program headlessly for --seconds at 12 fps and writes the last frame as PNG.
// Used to compare scenes against the reference implementation.
func render(args []string) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	secs := fs.Float64("seconds", 3, "simulated time to run before capturing")
	out := fs.String("out", "frame.png", "output PNG")
	data := fs.String("data", "", "data directory (default: a temporary one)")
	// Accept the program name before or after the flags.
	var name string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if name == "" && fs.NArg() > 0 {
		name = fs.Arg(0)
	}
	if name == "" {
		return errors.New("render NAME [--seconds N] [--out file.png]")
	}
	dir := *data
	if dir == "" {
		d, err := os.MkdirTemp("", "marquee-render")
		if err != nil {
			return err
		}
		defer os.RemoveAll(d)
		dir = d
	}
	img, err := engine.RenderOffline(dir, name, *secs)
	if err != nil {
		return err
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
