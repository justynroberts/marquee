// Command marquee drives Thermalright USB LED panels: a server with a web remote and HTTP API,
// a command-line client for that API, and a few direct panel diagnostics.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/justynroberts/marquee/internal/api"
	"github.com/justynroberts/marquee/internal/engine"
	"github.com/justynroberts/marquee/internal/panel"
)

var version = "dev"

const usage = `marquee: drive Thermalright USB LED panels

Server
  marquee app                            run as a macOS menu-bar app (the default inside Marquee.app)
  marquee [serve] [--addr 127.0.0.1:7781] [--lan] [--no-panel] [--data DIR] [--require-key]
      Run the panel server: web remote at http://127.0.0.1:7781, HTTP API under /v1.
      --lan listens on the network (other devices need an API key) and advertises via Bonjour.

Client (talks to a running server; MARQUEE_URL and MARQUEE_KEY override the defaults)
  marquee status                         what is playing
  marquee play NAME                      show, green, audio, board, dashboard, matrix, ...
  marquee flash TEXT [--seconds N] [--colour C]
  marquee say TEXT                       add a message to the board
  marquee timer MINUTES [LABEL]
  marquee hud on|off|clear|TEXT          heads-up mode, mirrored to read in your screen
  marquee hud mirror horizontal|vertical|off
  marquee hud lyrics on|off                 karaoke lyrics on the HUD instead of the clock
  marquee lyrics [earlier|later|SECONDS]   nudge lyric timing (0.25 s steps)
  marquee send FILE                      show an image (PNG, JPEG, GIF, WebP)
  marquee key create NAME | key list | key revoke ID   (on the server's machine)
  marquee mcp                            MCP server on stdin/stdout for Claude Code / Claude Desktop

Direct (no server running)
  marquee info | colour RRGGBB | image FILE | bench

  marquee version
`

func main() {
	var args []string
	for _, a := range os.Args[1:] {
		if !strings.HasPrefix(a, "-psn_") { // Finder's legacy process serial number
			args = append(args, a)
		}
	}
	cmd := "serve"
	if len(args) == 0 && inAppBundle() {
		cmd = "app"
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(args)
	case "app":
		err = runTray()
	case "render":
		err = render(args)
	case "mcp":
		err = runMCP(args)
	case "status", "play", "flash", "say", "timer", "send", "key", "hud", "lyrics":
		err = client(cmd, args)
	case "info", "colour", "color", "image", "bench":
		err = direct(cmd, args)
	case "version", "--version":
		fmt.Println("marquee", version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		err = fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "marquee:", err)
		os.Exit(1)
	}
}

// ---------------------------------------------------------------- server

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "", "listen address (default 127.0.0.1:7781, or :7781 with --lan)")
	lan := fs.Bool("lan", false, "listen on the network; other devices need an API key")
	noPanel := fs.Bool("no-panel", false, "do not touch USB; render and preview only")
	dataDir := fs.String("data", engine.DefaultDataDir(), "data directory")
	requireKey := fs.Bool("require-key", false, "require an API key even from this machine")
	sceneSecs := fs.Float64("scene-seconds", 20, "seconds per scene in playlists")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts := AppOptions{DataDir: *dataDir, NoPanel: *noPanel, SceneSeconds: *sceneSecs, Addr: *addr, Log: newLogger(nil)}
	// Flags given explicitly override the settings saved by the menu-bar app.
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "lan":
			opts.LAN = lan
		case "require-key":
			opts.RequireKey = requireKey
		}
	})
	api.Version = version
	a, err := NewApp(opts)
	if err != nil {
		return err
	}
	if err := a.Listen(); err != nil {
		a.Close()
		return err
	}
	a.Log.Info("marquee running", "version", version, "remote", a.RemoteURL(), "data", a.Paths.Dir)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-ctx.Done()
	a.Close()
	return nil
}

func newLogger(w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stderr
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func displayAddr(a string) string {
	if strings.HasPrefix(a, ":") {
		return "127.0.0.1" + a
	}
	return a
}

// ---------------------------------------------------------------- client

func baseURL() string {
	if u := os.Getenv("MARQUEE_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://127.0.0.1:7781"
}

func call(method, path string, body any, contentType string) (map[string]any, error) {
	var rdr io.Reader
	switch b := body.(type) {
	case nil:
	case []byte:
		rdr = bytes.NewReader(b)
	default:
		data, _ := json.Marshal(b)
		rdr = bytes.NewReader(data)
		contentType = "application/json"
	}
	req, _ := http.NewRequest(method, baseURL()+path, rdr)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if k := os.Getenv("MARQUEE_KEY"); k != "" {
		req.Header.Set("Authorization", "Bearer "+k)
	}
	res, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach Marquee at %s (is it running?): %w", baseURL(), err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode >= 300 {
		if msg, ok := out["error"].(string); ok {
			return out, errors.New(msg)
		}
		return out, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return out, nil
}

func client(cmd string, args []string) error {
	switch cmd {
	case "status":
		out, err := call("GET", "/v1/state", nil, "")
		if err != nil {
			return err
		}
		fmt.Printf("program %v  overlay %v  fps %v  brightness %v%%  panel %v\n", out["program"], or(out["overlay"], "-"), out["fps"], out["brightness"], out["panel"])
		if e, ok := out["error"].(string); ok && e != "" {
			fmt.Println("error:", e)
		}
		return nil
	case "play":
		if len(args) < 1 {
			return errors.New("play needs a program name (see GET /v1/programs)")
		}
		_, err := call("POST", "/v1/play", map[string]string{"program": args[0]}, "")
		return err
	case "lyrics":
		// marquee lyrics [earlier|later|SECONDS]
		body := map[string]any{}
		if len(args) > 0 {
			switch args[0] {
			case "earlier":
				body["adjust"] = 0.25
			case "later":
				body["adjust"] = -0.25
			default:
				v, err := strconv.ParseFloat(args[0], 64)
				if err != nil {
					return errors.New("usage: marquee lyrics [earlier|later|SECONDS]")
				}
				body["lead"] = v
			}
		}
		out, err := call("POST", "/v1/lyrics", body, "")
		if err == nil {
			fmt.Printf("lyrics appear %.2fs early\n", out["lead"])
		}
		return err
	case "hud":
		// marquee hud on | off | mirror horizontal|vertical|off | TEXT...
		body := map[string]any{}
		switch {
		case len(args) == 0:
			return errors.New("usage: marquee hud on | off | mirror horizontal|vertical|off | TEXT")
		case args[0] == "on" || args[0] == "off":
			body["on"] = args[0] == "on"
		case args[0] == "mirror" && len(args) == 2:
			body["mirror"] = args[1]
		case args[0] == "lyrics" && len(args) == 2 && (args[1] == "on" || args[1] == "off"):
			body["lyrics"] = args[1] == "on"
		case args[0] == "clear":
			body["text"] = ""
		default:
			body["text"] = strings.Join(args, " ")
		}
		out, err := call("POST", "/v1/hud", body, "")
		if err == nil {
			fmt.Printf("hud %v  mirror %v  lyrics %v  text %q\n", out["on"], out["mirror"], out["lyrics"], out["text"])
		}
		return err
	case "flash":
		fs := flag.NewFlagSet("flash", flag.ContinueOnError)
		secs := fs.Float64("seconds", 10, "how long")
		colour := fs.String("colour", "", "palette name or #rrggbb")
		text, rest := splitText(args)
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if text == "" {
			text = strings.Join(fs.Args(), " ")
		}
		_, err := call("POST", "/v1/flash", map[string]any{"text": text, "seconds": *secs, "colour": *colour}, "")
		return err
	case "say":
		_, err := call("POST", "/v1/messages", map[string]string{"text": strings.Join(args, " ")}, "")
		return err
	case "timer":
		if len(args) < 1 {
			return errors.New("timer needs minutes")
		}
		m, err := strconv.ParseFloat(args[0], 64)
		if err != nil {
			return errors.New("timer needs minutes")
		}
		_, err = call("POST", "/v1/timer", map[string]any{"minutes": m, "label": strings.Join(args[1:], " ")}, "")
		return err
	case "send":
		if len(args) < 1 {
			return errors.New("send needs an image file")
		}
		data, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		_, err = call("POST", "/v1/frame", data, http.DetectContentType(data))
		return err
	case "key":
		if len(args) == 0 {
			return errors.New("key create NAME | key list | key revoke ID")
		}
		switch args[0] {
		case "create":
			name := strings.Join(args[1:], " ")
			out, err := call("POST", "/v1/keys", map[string]string{"name": name}, "")
			if err != nil {
				return err
			}
			fmt.Println(out["secret"])
			fmt.Fprintln(os.Stderr, "Store this key now; it is not shown again.")
			return nil
		case "list":
			out, err := call("GET", "/v1/keys", nil, "")
			if err != nil {
				return err
			}
			for _, k := range out["keys"].([]any) {
				m := k.(map[string]any)
				fmt.Printf("%s  %-20v %v…  created %v  last used %v\n", m["id"], m["name"], m["prefix"], m["created"], or(m["lastUsed"], "never"))
			}
			return nil
		case "revoke":
			if len(args) < 2 {
				return errors.New("key revoke ID")
			}
			_, err := call("DELETE", "/v1/keys/"+args[1], nil, "")
			return err
		}
	}
	return fmt.Errorf("unknown %s command", cmd)
}

// splitText takes leading non-flag words as the text.
func splitText(args []string) (string, []string) {
	var words []string
	for i, a := range args {
		if strings.HasPrefix(a, "--") {
			return strings.Join(words, " "), args[i:]
		}
		words = append(words, a)
	}
	return strings.Join(words, " "), nil
}

func or(v any, d string) any {
	if v == nil || v == "" {
		return d
	}
	return v
}

// ---------------------------------------------------------------- direct panel access

func direct(cmd string, args []string) error {
	p, err := panel.Open()
	if err != nil {
		return fmt.Errorf("open panel: %w (if Marquee is running it owns the panel; use the client commands)", err)
	}
	defer p.Close()
	w, h := p.Size()
	switch cmd {
	case "info":
		fmt.Printf("%s  %dx%d  model %d  device id %08x\n", p.Model.Name, w, h, p.Info.Model, p.Info.DeviceID)
	case "colour", "color":
		var r, g, b uint8
		if len(args) < 1 {
			return errors.New("colour needs RRGGBB")
		}
		if _, err := fmt.Sscanf(strings.TrimPrefix(args[0], "#"), "%02x%02x%02x", &r, &g, &b); err != nil {
			return fmt.Errorf("bad colour %q", args[0])
		}
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{r, g, b, 255}}, image.Point{}, draw.Src)
		return p.SendImage(img)
	case "image":
		if len(args) < 1 {
			return errors.New("image needs a file")
		}
		f, err := os.Open(args[0])
		if err != nil {
			return err
		}
		defer f.Close()
		src, _, err := image.Decode(f)
		if err != nil {
			return err
		}
		return p.SendImage(engine.FitCover(src))
	case "bench":
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		start := time.Now()
		n := 60
		for i := 0; i < n; i++ {
			draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{12, 13, 11, 255}}, image.Point{}, draw.Src)
			x := i * (w - 120) / n
			draw.Draw(img, image.Rect(x, 100, x+120, 362), &image.Uniform{color.RGBA{255, 91, 56, 255}}, image.Point{}, draw.Src)
			if err := p.SendImage(img); err != nil {
				return err
			}
		}
		d := time.Since(start)
		fmt.Printf("%d frames in %s: %.1f fps\n", n, d.Round(time.Millisecond), float64(n)/d.Seconds())
	}
	return nil
}
