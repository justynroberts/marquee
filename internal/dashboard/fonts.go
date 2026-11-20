package dashboard

import (
	"bytes"
	"embed"
	"os"
	"path/filepath"
	"sync"
)

// Two handwriting faces ship inside the binary for the diary screens: Homemade Apple (Apache
// 2.0) for the writing and Meddon (SIL OFL 1.1) for dates and big figures. Licences are beside
// them in fonts/. FreeType opens fonts by path, so they are copied to the cache folder once.

//go:embed fonts/*.ttf
var embeddedFonts embed.FS

var (
	embedMu    sync.Mutex
	embedPaths = map[string]string{}
)

// embeddedFontPath returns a file path for a bundled font, writing it out on first use.
func embeddedFontPath(name string) string {
	embedMu.Lock()
	defer embedMu.Unlock()
	if p, ok := embedPaths[name]; ok {
		return p
	}
	b, err := embeddedFonts.ReadFile("fonts/" + name)
	if err != nil {
		return ""
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "Marquee", "fonts")
	p := filepath.Join(dir, name)
	if old, err := os.ReadFile(p); err != nil || !bytes.Equal(old, b) {
		if os.MkdirAll(dir, 0o755) != nil || os.WriteFile(p, b, 0o644) != nil {
			return ""
		}
	}
	embedPaths[name] = p
	return p
}
