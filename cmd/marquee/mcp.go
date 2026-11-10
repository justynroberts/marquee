package main

// marquee mcp: a Model Context Protocol server on stdin/stdout so Claude Code, Claude Desktop and
// other MCP clients can drive the panel. It is a thin client of the running server's HTTP API
// (MARQUEE_URL / MARQUEE_KEY, like the other client commands); it never touches USB itself.
// Transport: newline-delimited JSON-RPC 2.0. stdout carries protocol messages only; logs go to stderr.

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	mcpDefaultProtocol = "2025-06-18"
	mcpMaxImage        = 10 << 20
	mcpFetchTimeout    = 15 * time.Second
)

var mcpProtocols = map[string]bool{"2025-06-18": true, "2025-03-26": true, "2024-11-05": true}

// JSON-RPC error codes.
const (
	rpcParseError     = -32700
	rpcInvalidRequest = -32600
	rpcMethodNotFound = -32601
	rpcInvalidParams  = -32602
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string
	ID      json.RawMessage
	Result  any
	Error   *rpcError
}

// MarshalJSON emits exactly one of result or error (an empty result object is still sent).
func (r rpcResponse) MarshalJSON() ([]byte, error) {
	if r.Error != nil {
		return json.Marshal(struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Error   *rpcError       `json:"error"`
		}{"2.0", r.ID, r.Error})
	}
	return json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  any             `json:"result"`
	}{"2.0", r.ID, r.Result})
}

// mcpTool is one tool: its advertised definition and its handler. A handler returns MCP content
// blocks; an error becomes a tool result with isError set (not a JSON-RPC error).
type mcpTool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	run         func(args json.RawMessage) ([]any, error)
}

func runMCP(args []string) error {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintln(os.Stderr, "marquee mcp: MCP server on stdin/stdout for Claude Code and Claude Desktop; talks to the server at", baseURL())
		return nil
	}
	log := newLogger(os.Stderr)
	log.Info("marquee mcp started", "version", version, "server", baseURL())
	return serveMCP(os.Stdin, os.Stdout, log)
}

// serveMCP answers requests from in until EOF.
func serveMCP(in io.Reader, out io.Writer, log *slog.Logger) error {
	tools := mcpTools()
	byName := make(map[string]*mcpTool, len(tools))
	for i := range tools {
		byName[tools[i].Name] = &tools[i]
	}
	w := bufio.NewWriter(out)
	send := func(v any) {
		data, err := json.Marshal(v)
		if err != nil {
			log.Error("encode response", "err", err)
			return
		}
		w.Write(data)
		w.WriteByte('\n')
		w.Flush()
	}
	r := bufio.NewReaderSize(in, 1<<16)
	for {
		line, err := r.ReadBytes('\n')
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			if line[0] == '[' { // a batch (2025-03-26 and earlier)
				var batch []json.RawMessage
				if jerr := json.Unmarshal(line, &batch); jerr != nil || len(batch) == 0 {
					send(rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{rpcInvalidRequest, "invalid batch"}})
				} else {
					var replies []rpcResponse
					for _, m := range batch {
						if resp := handleMCP(m, byName, tools, log); resp != nil {
							replies = append(replies, *resp)
						}
					}
					if len(replies) > 0 {
						send(replies)
					}
				}
			} else if resp := handleMCP(line, byName, tools, log); resp != nil {
				send(resp)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// handleMCP handles one message; it returns nil for notifications (and responses from the client).
func handleMCP(raw []byte, byName map[string]*mcpTool, tools []mcpTool, log *slog.Logger) *rpcResponse {
	var req rpcRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return &rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{rpcParseError, "parse error: " + err.Error()}}
	}
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"
	if req.Method == "" {
		if isNotification {
			return nil // a response to something we never sent, or junk without an id
		}
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{rpcInvalidRequest, "method is required"}}
	}
	if isNotification {
		log.Debug("notification", "method", req.Method)
		return nil
	}
	reply := func(result any) *rpcResponse { return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result} }
	fail := func(code int, msg string) *rpcResponse {
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{code, msg}}
	}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
			ClientInfo      struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"clientInfo"`
		}
		_ = json.Unmarshal(req.Params, &p)
		pv := mcpDefaultProtocol
		if mcpProtocols[p.ProtocolVersion] {
			pv = p.ProtocolVersion
		}
		log.Info("initialize", "client", p.ClientInfo.Name, "clientVersion", p.ClientInfo.Version, "protocol", pv)
		return reply(map[string]any{
			"protocolVersion": pv,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "marquee", "version": version},
			"instructions": "Marquee drives a 1920x462 USB LED panel (a 275x66 grid of round LEDs). Use panel_status and preview to see what is showing, " +
				"list_programs then play to switch programs, flash for short alerts, say/set_messages for the scrolling board.",
		})
	case "ping":
		return reply(map[string]any{})
	case "tools/list":
		return reply(map[string]any{"tools": tools})
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return fail(rpcInvalidParams, "params must be {name, arguments}")
		}
		t, ok := byName[p.Name]
		if !ok {
			return reply(toolError(fmt.Errorf("unknown tool %q", p.Name)))
		}
		args := p.Arguments
		if len(args) == 0 || string(args) == "null" {
			args = json.RawMessage("{}")
		}
		start := time.Now()
		content, err := t.run(args)
		log.Info("tool", "name", p.Name, "ms", time.Since(start).Milliseconds(), "ok", err == nil)
		if err != nil {
			return reply(toolError(err))
		}
		return reply(map[string]any{"content": content})
	default:
		return fail(rpcMethodNotFound, "method not found: "+req.Method)
	}
}

func toolError(err error) map[string]any {
	return map[string]any{"content": []any{textContent(err.Error())}, "isError": true}
}

func textContent(s string) map[string]any { return map[string]any{"type": "text", "text": s} }

func jsonContent(v any) []any {
	data, _ := json.MarshalIndent(v, "", "  ")
	return []any{textContent(string(data))}
}

// decodeArgs unmarshals tool arguments strictly enough to catch type mistakes.
func decodeArgs(raw json.RawMessage, v any) error {
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("invalid arguments: %v", err)
	}
	return nil
}

// ---------------------------------------------------------------- schemas

func schema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func prop(typ, desc string) map[string]any { return map[string]any{"type": typ, "description": desc} }

const boardCheatSheet = `

Board markup (one message per line; tags open a style, [/] closes the latest):
- Colours: [red] [orange] [yellow] [green] [cyan] [blue] [purple] [pink] [white] [grey] [alert] [black], or [#ff00aa] / [#f0a] / [colour=...]
- Background: [bg=blue] or [bg=#102030]
- Effects (combine): [rainbow] [blink] [pulse] [wave] [shimmer]; [plain] clears effects
- Size: [big] [small] [tiny] [size=30] (8-66, in LED rows; default 44)
- Fonts: [sans] [serif] [mono] [script] [condensed] [rounded] [typewriter] [chalk] [stencil] [didot] [futura] [marker] [hand] [poster] [cjk] [pixel], or [font=Any Installed Font]
- Sprites (animated): :heart: :fire: :star: :ufo: :wave: :check: :cross:   Unicode emoji work too: 🚀 🎉 ☕️
- Live tokens: {time} {date} {day} {cpu} {mem} {weather} {temp} {incidents} {incident}
- A line starting with ! is an alert (vermilion, pulsing)
- Board-wide settings on their own line (set_messages only): @step=2 (scroll speed 1-40) @gap=34 @font=serif @size=44 @colour=white @dim=#1e1e1a @bg=#0c0d0b @theme=green|amber (phosphor look) @location=London (for {weather})
- # starts a comment line
Example: [wave][green]Build passed[/][/] :check:   [bg=blue][white] ON AIR [/][/]  [big]{time}[/]`

func mcpTools() []mcpTool {
	empty := schema(map[string]any{})
	return []mcpTool{
		{
			Name: "panel_status", Title: "Panel status",
			Description: "What the LED panel is doing now: current program, brightness (%), paused, flipped, active overlay (flash/timer), frames per second, and whether the USB panel is connected.",
			InputSchema: empty,
			run: func(json.RawMessage) ([]any, error) {
				out, err := call("GET", "/v1/state", nil, "")
				if err != nil {
					return nil, err
				}
				return jsonContent(out), nil
			},
		},
		{
			Name: "list_programs", Title: "List programs",
			Description: "List every program the panel can play, grouped (playlist, panel, scene groups, media). Pass a name to play.",
			InputSchema: empty,
			run: func(json.RawMessage) ([]any, error) {
				out, err := call("GET", "/v1/programs", nil, "")
				if err != nil {
					return nil, err
				}
				groups := map[string][]map[string]any{}
				list, _ := out["programs"].([]any)
				for _, p := range list {
					m, ok := p.(map[string]any)
					if !ok {
						continue
					}
					g, _ := m["group"].(string)
					if g == "" {
						g = "other"
					}
					groups[g] = append(groups[g], map[string]any{"name": m["name"], "description": m["description"]})
				}
				names := make([]string, 0, len(groups))
				for g := range groups {
					names = append(names, g)
				}
				sort.Strings(names)
				var b strings.Builder
				fmt.Fprintf(&b, "%d programs in %d groups\n", len(list), len(groups))
				for _, g := range names {
					fmt.Fprintf(&b, "\n## %s\n", g)
					for _, p := range groups[g] {
						fmt.Fprintf(&b, "- %v", p["name"])
						if d, _ := p["description"].(string); d != "" && d != p["name"] {
							fmt.Fprintf(&b, ": %s", d)
						}
						b.WriteByte('\n')
					}
				}
				return []any{textContent(b.String())}, nil
			},
		},
		{
			Name: "play", Title: "Play a program",
			Description: "Switch the panel to a program by name (from list_programs), e.g. \"board\", \"showcase\", a scene name, or \"media:<file>\".",
			InputSchema: schema(map[string]any{"name": prop("string", "Program name exactly as listed by list_programs.")}, "name"),
			run: func(raw json.RawMessage) ([]any, error) {
				var a struct {
					Name string `json:"name"`
				}
				if err := decodeArgs(raw, &a); err != nil {
					return nil, err
				}
				if strings.TrimSpace(a.Name) == "" {
					return nil, errors.New("name is required (see list_programs)")
				}
				if _, err := call("POST", "/v1/play", map[string]string{"program": a.Name}, ""); err != nil {
					return nil, err
				}
				return []any{textContent("Now playing " + a.Name)}, nil
			},
		},
		{
			Name: "flash", Title: "Flash an alert",
			Description: "Show a short alert over whatever is playing, then return to it. Emoji are supported. Text is cut to 300 characters.",
			InputSchema: schema(map[string]any{
				"text":    prop("string", "Alert text (emoji supported, max 300 characters)."),
				"seconds": map[string]any{"type": "number", "description": "How long to show it, 1-600 seconds (default 10).", "minimum": 1, "maximum": 600},
				"colour":  prop("string", "Palette name (red, orange, yellow, green, cyan, blue, purple, pink, white, grey, alert) or #rrggbb. Default: alert vermilion."),
			}, "text"),
			run: func(raw json.RawMessage) ([]any, error) {
				var a struct {
					Text    string   `json:"text"`
					Seconds *float64 `json:"seconds"`
					Colour  string   `json:"colour"`
				}
				if err := decodeArgs(raw, &a); err != nil {
					return nil, err
				}
				if strings.TrimSpace(a.Text) == "" {
					return nil, errors.New("text is required")
				}
				body := map[string]any{"text": a.Text, "colour": a.Colour}
				if a.Seconds != nil {
					body["seconds"] = *a.Seconds
				}
				if _, err := call("POST", "/v1/flash", body, ""); err != nil {
					return nil, err
				}
				return []any{textContent("Flashed: " + a.Text)}, nil
			},
		},
		{
			Name: "say", Title: "Add a board message",
			Description: "Append one message line to the scrolling message board (play \"board\" to show it). Newlines are flattened; max 500 characters." + boardCheatSheet,
			InputSchema: schema(map[string]any{"text": prop("string", "One message line, using board markup.")}, "text"),
			run: func(raw json.RawMessage) ([]any, error) {
				var a struct {
					Text string `json:"text"`
				}
				if err := decodeArgs(raw, &a); err != nil {
					return nil, err
				}
				out, err := call("POST", "/v1/messages", map[string]string{"text": a.Text}, "")
				if err != nil {
					return nil, err
				}
				return withWarnings("Added to the board.", out), nil
			},
		},
		{
			Name: "set_messages", Title: "Replace the board",
			Description: "Replace the whole message board file (messages.txt) with new text: one message per line, @settings and # comments allowed, up to 200 KB. Call get_messages first if you want to keep existing lines." + boardCheatSheet,
			InputSchema: schema(map[string]any{"text": prop("string", "The complete new board file.")}, "text"),
			run: func(raw json.RawMessage) ([]any, error) {
				var a struct {
					Text string `json:"text"`
				}
				if err := decodeArgs(raw, &a); err != nil {
					return nil, err
				}
				out, err := call("PUT", "/v1/messages", map[string]string{"text": a.Text}, "")
				if err != nil {
					return nil, err
				}
				return withWarnings("Board replaced.", out), nil
			},
		},
		{
			Name: "get_messages", Title: "Read the board",
			Description: "Return the message board file (messages.txt) as text.",
			InputSchema: empty,
			run: func(json.RawMessage) ([]any, error) {
				out, err := call("GET", "/v1/messages", nil, "")
				if err != nil {
					return nil, err
				}
				s, _ := out["text"].(string)
				return []any{textContent(s)}, nil
			},
		},
		{
			Name: "list_playlists", Title: "List playlists",
			Description: "List built-in and custom playlists with their items.",
			InputSchema: empty,
			run: func(json.RawMessage) ([]any, error) {
				out, err := call("GET", "/v1/playlists", nil, "")
				if err != nil {
					return nil, err
				}
				return jsonContent(out), nil
			},
		},
		{
			Name: "set_playlist", Title: "Create or replace a playlist",
			Description: "Create or replace a custom playlist, then play it by name. Items are program names: scenes (matrix, fire, plasma, clock, ...), board (one pass of the message board), news (one pass of the headline ticker), nowplaying, dashboard, interference, bigclock, worldclock, headlines, newsreel (or newsreel:bbc, newsreel:sky, newsreel:hn, ...), flights, quakes, onthisday, moon, countdown, wordclock, calendar, markets, air, weather, ascii, or media:FILE. Each non-board item plays for `seconds`.",
			InputSchema: schema(map[string]any{
				"name":    prop("string", "Lowercase name: letters, digits, - and _ (not a built-in program name)."),
				"items":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Program names in order.", "minItems": 1, "maxItems": 100},
				"seconds": map[string]any{"type": "number", "description": "Seconds per item (default 20).", "minimum": 0, "maximum": 3600},
			}, "name", "items"),
			run: func(raw json.RawMessage) ([]any, error) {
				var a struct {
					Name    string   `json:"name"`
					Items   []string `json:"items"`
					Seconds float64  `json:"seconds"`
				}
				if err := decodeArgs(raw, &a); err != nil {
					return nil, err
				}
				if a.Name == "" {
					return nil, fmt.Errorf("name is required")
				}
				if _, err := call("PUT", "/v1/playlists/"+url.PathEscape(a.Name), map[string]any{"items": a.Items, "seconds": a.Seconds}, ""); err != nil {
					return nil, err
				}
				return []any{textContent("Saved playlist " + a.Name + ". Play it with play {name: \"" + a.Name + "\"}.")}, nil
			},
		},
		{
			Name: "delete_playlist", Title: "Delete a playlist",
			Description: "Delete a custom playlist (built-ins cannot be deleted).",
			InputSchema: schema(map[string]any{"name": prop("string", "The custom playlist name.")}, "name"),
			run: func(raw json.RawMessage) ([]any, error) {
				var a struct {
					Name string `json:"name"`
				}
				if err := decodeArgs(raw, &a); err != nil {
					return nil, err
				}
				if _, err := call("DELETE", "/v1/playlists/"+url.PathEscape(a.Name), nil, ""); err != nil {
					return nil, err
				}
				return []any{textContent("Deleted playlist " + a.Name + ".")}, nil
			},
		},
		{
			Name: "refresh_news", Title: "Refresh news headlines",
			Description: "Fetch headlines for the news ticker now (Hacker News by default, or the RSS/Atom feeds listed in news-sources.txt in the data folder). Play \"news\" to show them.",
			InputSchema: empty,
			run: func(json.RawMessage) ([]any, error) {
				if _, err := call("POST", "/v1/news/refresh", nil, ""); err != nil {
					return nil, err
				}
				return []any{textContent("Headlines refreshed.")}, nil
			},
		},
		{
			Name: "hud", Title: "Heads-up display",
			Description: "HUD mode: a plain, high-contrast screen (huge clock plus one line: the HUD text, the playing track, or the date; flashes appear full-screen in amber), mirrored so it reads correctly reflected in a monitor. Set any of on, text, mirror.",
			InputSchema: schema(map[string]any{
				"on":     prop("boolean", "Turn HUD mode on (plays the HUD, mirrors horizontally) or off (no mirror, back to the previous program)."),
				"text":   prop("string", "The HUD message line, up to 120 characters; empty string clears it."),
				"lyrics": prop("boolean", "Show synced karaoke lyrics of the playing song instead of the clock (falls back to the clock when the song has none)."),
				"mirror": map[string]any{"type": "string", "enum": []string{"horizontal", "vertical", "off"}, "description": "horizontal: panel faces the screen; vertical: panel lies below the screen facing up."},
			}),
			run: func(raw json.RawMessage) ([]any, error) {
				var a map[string]any
				if err := decodeArgs(raw, &a); err != nil {
					return nil, err
				}
				out, err := call("POST", "/v1/hud", a, "")
				if err != nil {
					return nil, err
				}
				return jsonContent(out), nil
			},
		},
		{
			Name: "list_fonts", Title: "List fonts",
			Description: "List font shortcut names usable as [name] board tags, and installed font names usable as [font=Name].",
			InputSchema: empty,
			run: func(json.RawMessage) ([]any, error) {
				out, err := call("GET", "/v1/fonts", nil, "")
				if err != nil {
					return nil, err
				}
				return jsonContent(out), nil
			},
		},
		{
			Name: "timer", Title: "Start a countdown",
			Description: "Start a countdown timer overlay on the panel.",
			InputSchema: schema(map[string]any{
				"minutes": map[string]any{"type": "number", "description": "Duration in minutes (fractions allowed), up to 1440.", "exclusiveMinimum": 0, "maximum": 1440},
				"label":   prop("string", "Optional label shown with the timer (max 40 characters)."),
			}, "minutes"),
			run: func(raw json.RawMessage) ([]any, error) {
				var a struct {
					Minutes *float64 `json:"minutes"`
					Label   string   `json:"label"`
				}
				if err := decodeArgs(raw, &a); err != nil {
					return nil, err
				}
				if a.Minutes == nil {
					return nil, errors.New("minutes is required")
				}
				if _, err := call("POST", "/v1/timer", map[string]any{"minutes": *a.Minutes, "label": a.Label}, ""); err != nil {
					return nil, err
				}
				return []any{textContent(fmt.Sprintf("Timer started: %g minutes", *a.Minutes))}, nil
			},
		},
		{
			Name: "clear_alert", Title: "Clear alert",
			Description: "Remove the current flash or timer overlay.",
			InputSchema: empty,
			run: func(json.RawMessage) ([]any, error) {
				if _, err := call("DELETE", "/v1/overlay", nil, ""); err != nil {
					return nil, err
				}
				return []any{textContent("Overlay cleared.")}, nil
			},
		},
		{
			Name: "brightness", Title: "Set brightness",
			Description: "Set panel brightness in percent (clamped to 5-100).",
			InputSchema: schema(map[string]any{"percent": map[string]any{"type": "integer", "description": "Brightness percent, 5-100.", "minimum": 5, "maximum": 100}}, "percent"),
			run: func(raw json.RawMessage) ([]any, error) {
				var a struct {
					Percent *float64 `json:"percent"`
				}
				if err := decodeArgs(raw, &a); err != nil {
					return nil, err
				}
				if a.Percent == nil {
					return nil, errors.New("percent is required")
				}
				out, err := call("POST", "/v1/brightness", map[string]any{"percent": int(*a.Percent)}, "")
				if err != nil {
					return nil, err
				}
				return []any{textContent(fmt.Sprintf("Brightness %v%%", out["percent"]))}, nil
			},
		},
		{
			Name: "pause", Title: "Pause or resume",
			Description: "Pause (freeze the current frame) or resume the panel.",
			InputSchema: schema(map[string]any{"paused": prop("boolean", "true to pause, false to resume.")}, "paused"),
			run: func(raw json.RawMessage) ([]any, error) {
				var a struct {
					Paused *bool `json:"paused"`
				}
				if err := decodeArgs(raw, &a); err != nil {
					return nil, err
				}
				if a.Paused == nil {
					return nil, errors.New("paused is required")
				}
				if _, err := call("POST", "/v1/pause", map[string]any{"paused": *a.Paused}, ""); err != nil {
					return nil, err
				}
				if *a.Paused {
					return []any{textContent("Paused.")}, nil
				}
				return []any{textContent("Resumed.")}, nil
			},
		},
		{
			Name: "flip", Title: "Flip the display",
			Description: "Rotate the output 180 degrees (for a panel mounted upside down).",
			InputSchema: schema(map[string]any{"flipped": prop("boolean", "true to rotate 180 degrees, false for normal orientation.")}, "flipped"),
			run: func(raw json.RawMessage) ([]any, error) {
				var a struct {
					Flipped *bool `json:"flipped"`
				}
				if err := decodeArgs(raw, &a); err != nil {
					return nil, err
				}
				if a.Flipped == nil {
					return nil, errors.New("flipped is required")
				}
				if _, err := call("POST", "/v1/flip", map[string]any{"flip": *a.Flipped}, ""); err != nil {
					return nil, err
				}
				return []any{textContent(fmt.Sprintf("Flip %v.", *a.Flipped))}, nil
			},
		},
		{
			Name: "show_image", Title: "Show an image",
			Description: "Show a still image (PNG, JPEG, GIF or WebP) on the panel, cropped to fill the 1920x462 panel. Give exactly one of path (a local file) or url (fetched over http/https). Max 10 MB.",
			InputSchema: schema(map[string]any{
				"path": prop("string", "Absolute path of a local image file."),
				"url":  prop("string", "http(s) URL of an image to fetch (15 s timeout)."),
			}),
			run: func(raw json.RawMessage) ([]any, error) {
				var a struct {
					Path string `json:"path"`
					URL  string `json:"url"`
				}
				if err := decodeArgs(raw, &a); err != nil {
					return nil, err
				}
				data, src, err := loadImage(a.Path, a.URL)
				if err != nil {
					return nil, err
				}
				if _, err := call("POST", "/v1/frame", data, http.DetectContentType(data)); err != nil {
					return nil, err
				}
				return []any{textContent("Showing " + src)}, nil
			},
		},
		{
			Name: "preview", Title: "See the panel",
			Description: "Return a JPEG snapshot of exactly what the panel is showing right now, so you can see it.",
			InputSchema: empty,
			run: func(json.RawMessage) ([]any, error) {
				data, ct, err := getRaw("/v1/preview.jpg")
				if err != nil {
					return nil, err
				}
				if ct == "" || !strings.HasPrefix(ct, "image/") {
					ct = "image/jpeg"
				}
				return []any{map[string]any{"type": "image", "data": base64.StdEncoding.EncodeToString(data), "mimeType": ct}}, nil
			},
		},
	}
}

func withWarnings(msg string, out map[string]any) []any {
	if ws, ok := out["warnings"].([]any); ok && len(ws) > 0 {
		var b strings.Builder
		b.WriteString(msg + "\nWarnings:")
		for _, w := range ws {
			fmt.Fprintf(&b, "\n- %v", w)
		}
		return []any{textContent(b.String())}
	}
	return []any{textContent(msg)}
}

// getRaw GETs a non-JSON resource from the server with the same auth as call.
func getRaw(path string) ([]byte, string, error) {
	req, _ := http.NewRequest("GET", baseURL()+path, nil)
	if k := os.Getenv("MARQUEE_KEY"); k != "" {
		req.Header.Set("Authorization", "Bearer "+k)
	}
	res, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("cannot reach Marquee at %s (is it running?): %w", baseURL(), err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, mcpMaxImage+1))
	if err != nil {
		return nil, "", err
	}
	if res.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return nil, "", errors.New(e.Error)
		}
		return nil, "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return data, res.Header.Get("Content-Type"), nil
}

// loadImage reads a local file or fetches a URL, capped at mcpMaxImage.
func loadImage(path, url string) ([]byte, string, error) {
	switch {
	case path != "" && url != "":
		return nil, "", errors.New("give either path or url, not both")
	case path != "":
		st, err := os.Stat(path)
		if err != nil {
			return nil, "", err
		}
		if st.IsDir() {
			return nil, "", fmt.Errorf("%s is a directory", path)
		}
		if st.Size() > mcpMaxImage {
			return nil, "", errors.New("image is larger than 10 MB")
		}
		data, err := os.ReadFile(path)
		return data, path, err
	case url != "":
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			return nil, "", errors.New("url must be http or https")
		}
		res, err := (&http.Client{Timeout: mcpFetchTimeout}).Get(url)
		if err != nil {
			return nil, "", fmt.Errorf("fetch %s: %w", url, err)
		}
		defer res.Body.Close()
		if res.StatusCode >= 300 {
			return nil, "", fmt.Errorf("fetch %s: HTTP %d", url, res.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(res.Body, mcpMaxImage+1))
		if err != nil {
			return nil, "", fmt.Errorf("fetch %s: %w", url, err)
		}
		if len(data) > mcpMaxImage {
			return nil, "", errors.New("image is larger than 10 MB")
		}
		return data, url, nil
	}
	return nil, "", errors.New("give path (a local file) or url")
}
