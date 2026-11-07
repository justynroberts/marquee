// Package scenes holds every animated program Marquee can play. A Scene draws one frame at
// LED resolution; the engine expands it to the panel image and sends it.
package scenes

import (
	"sort"

	"github.com/justynroberts/marquee/internal/led"
)

// Scene draws frames. t is seconds since the scene started, dt seconds since the last frame.
type Scene interface {
	Draw(g *led.Grid, t, dt float64)
}

// Factory makes a fresh scene.
type Factory func() Scene

// Info describes a registered scene.
type Info struct {
	Name        string
	Group       string // "scene" | "audio"
	Description string
	New         Factory
}

var registry = map[string]Info{}

// Register adds a scene. Called from init() in each scene file.
func Register(name, group, description string, f Factory) {
	registry[name] = Info{Name: name, Group: group, Description: description, New: f}
}

// Get returns a scene by name.
func Get(name string) (Info, bool) {
	i, ok := registry[name]
	return i, ok
}

// List returns every registered scene sorted by group then name.
func List() []Info {
	out := make([]Info, 0, len(registry))
	for _, i := range registry {
		out = append(out, i)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Group != out[b].Group {
			return out[a].Group < out[b].Group
		}
		return out[a].Name < out[b].Name
	})
	return out
}
