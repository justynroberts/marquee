package api

import "net/http"

// openapi serves a hand-written OpenAPI 3.1 description of /v1.
func (s *Server) openapi(w http.ResponseWriter, r *http.Request) {
	obj := func(props map[string]any, required ...string) map[string]any {
		m := map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			m["required"] = required
		}
		return m
	}
	str := map[string]any{"type": "string"}
	num := map[string]any{"type": "number"}
	integer := map[string]any{"type": "integer"}
	boolean := map[string]any{"type": "boolean"}
	jsonBody := func(schema map[string]any) map[string]any {
		return map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": schema}}}
	}
	ok := map[string]any{"200": map[string]any{"description": "OK", "content": map[string]any{"application/json": map[string]any{"schema": obj(map[string]any{"ok": boolean})}}}}
	op := func(summary string, body map[string]any) map[string]any {
		m := map[string]any{"summary": summary, "responses": ok}
		if body != nil {
			m["requestBody"] = body
		}
		return m
	}
	image := map[string]any{"required": true, "content": map[string]any{
		"image/png":           map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}},
		"image/jpeg":          map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}},
		"image/gif":           map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}},
		"multipart/form-data": map[string]any{"schema": obj(map[string]any{"file": map[string]any{"type": "string", "format": "binary"}})},
	}}
	textBody := jsonBody(obj(map[string]any{"text": str}, "text"))
	spec := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   "Marquee API",
			"version": Version,
			"description": "Drive a Thermalright USB LED panel. Requests from the machine running Marquee need no key. " +
				"Requests from other devices need an API key (create one in the Marquee menu or POST /v1/keys on that machine) " +
				"sent as `Authorization: Bearer mq_...` (or `?key=` for <img> and EventSource). Marquee advertises itself " +
				"over Bonjour as `_marquee._tcp` when network access is on.",
		},
		"components": map[string]any{"securitySchemes": map[string]any{"bearer": map[string]any{"type": "http", "scheme": "bearer"}}},
		"security":   []any{map[string]any{"bearer": []any{}}},
		"paths": map[string]any{
			"/v1":              map[string]any{"get": op("Server, version and panel information", nil)},
			"/v1/state":        map[string]any{"get": op("Current program, overlay, brightness, fps, panel status", nil)},
			"/v1/programs":     map[string]any{"get": op("Everything /v1/play accepts: playlists, panels, scenes, audio visualisers, media", nil)},
			"/v1/play":         map[string]any{"post": op("Switch program", jsonBody(obj(map[string]any{"program": map[string]any{"type": "string", "examples": []string{"show", "matrix", "board", "dashboard", "audio", "media:party.gif"}}}, "program")))},
			"/v1/flash":        map[string]any{"post": op("Show an alert over whatever is playing, then return to it", jsonBody(obj(map[string]any{"text": str, "seconds": num, "colour": map[string]any{"type": "string", "description": "palette name or #rrggbb"}}, "text")))},
			"/v1/timer":        map[string]any{"post": op("Start a countdown overlay", jsonBody(obj(map[string]any{"minutes": num, "seconds": num, "label": str})))},
			"/v1/overlay":      map[string]any{"delete": op("Cancel a running flash or timer", nil)},
			"/v1/messages":     map[string]any{"get": op("messages.txt", nil), "put": op("Replace messages.txt (returns lint warnings)", textBody), "post": op("Append one message line (tags, emoji, :sprites:, {tokens} allowed)", textBody)},
			"/v1/schedule":     map[string]any{"get": op("schedule.txt and its parsed entries", nil), "put": op("Replace schedule.txt", textBody)},
			"/v1/media":        map[string]any{"get": op("Uploaded pictures and GIFs", nil), "post": op("Upload a picture or animated GIF and play it (multipart field file, or a raw image body with ?name=)", image)},
			"/v1/media/{name}": map[string]any{"delete": map[string]any{"summary": "Delete uploaded media", "parameters": []any{map[string]any{"name": "name", "in": "path", "required": true, "schema": str}}, "responses": ok}},
			"/v1/frame":        map[string]any{"post": op("Push one image to show now (cropped to fill 1920x462). Post repeatedly to stream from your app (up to ~12 fps).", image)},
			"/v1/draw":         map[string]any{"post": op("Set the drawing-pad canvas (275x66 LEDs) from a PNG data URL", jsonBody(obj(map[string]any{"png": str}, "png")))},
			"/v1/brightness":   map[string]any{"post": op("Software brightness 5-100", jsonBody(obj(map[string]any{"percent": integer}, "percent")))},
			"/v1/pause":        map[string]any{"post": op("Stop or resume sending frames", jsonBody(obj(map[string]any{"paused": boolean})))},
			"/v1/flip":         map[string]any{"post": op("Turn the image upside down (panel mounted the other way up)", jsonBody(obj(map[string]any{"flip": boolean})))},
			"/v1/preview.jpg":  map[string]any{"get": map[string]any{"summary": "The frame on the panel now", "responses": map[string]any{"200": map[string]any{"description": "JPEG", "content": map[string]any{"image/jpeg": map[string]any{}}}}}},
			"/v1/events":       map[string]any{"get": map[string]any{"summary": "Server-sent events: state, overlay, panel, error", "responses": map[string]any{"200": map[string]any{"description": "text/event-stream", "content": map[string]any{"text/event-stream": map[string]any{}}}}}},
			"/v1/fonts":        map[string]any{"get": op("Font shortcuts and installed fonts usable in [font=...]", nil)},
			"/v1/keys":         map[string]any{"get": op("List API keys (local only)", nil), "post": op("Create an API key; the secret is returned once (local only)", jsonBody(obj(map[string]any{"name": str})))},
			"/v1/playlists":    map[string]any{"get": op("Built-in and custom playlists", nil)},
			"/v1/playlists/{name}": map[string]any{
				"put":    map[string]any{"summary": "Create or replace a custom playlist. Items: scene names, board, news, nowplaying, dashboard, media:FILE. seconds per item (0 = default)", "parameters": []any{map[string]any{"name": "name", "in": "path", "required": true, "schema": str}}, "requestBody": jsonBody(obj(map[string]any{"items": map[string]any{"type": "array", "items": str}, "seconds": num}, "items")), "responses": ok},
				"delete": map[string]any{"summary": "Delete a custom playlist", "parameters": []any{map[string]any{"name": "name", "in": "path", "required": true, "schema": str}}, "responses": ok},
			},
			"/v1/hud":            map[string]any{"post": op("HUD mode: lyrics shows synced lyrics instead of the clock while a song has them; on (big clock and one line, high contrast, mirrored to read in a screen's reflection), text (the message line, empty clears), mirror", jsonBody(obj(map[string]any{"on": boolean, "text": str, "lyrics": boolean, "mirror": map[string]any{"type": "string", "enum": []string{"horizontal", "vertical", "off"}}})))},
			"/v1/mirror":         map[string]any{"post": op("Mirror everything sent to the panel: horizontal, vertical or off", jsonBody(obj(map[string]any{"mirror": map[string]any{"type": "string", "enum": []string{"horizontal", "vertical", "off"}}}, "mirror")))},
			"/v1/lyrics":         map[string]any{"post": op("Lyric timing: lead sets how many seconds early lines appear (default 1.1); adjust nudges it (+ earlier, - later). Empty body reports it", jsonBody(obj(map[string]any{"lead": num, "adjust": num})))},
			"/v1/update":         map[string]any{"get": op("Update check against GitHub Releases: current, latest, available. ?check=1 checks now", nil)},
			"/v1/news/refresh":   map[string]any{"post": op("Fetch headlines for the news ticker now (Hacker News, or the feeds in news-sources.txt)", nil)},
			"/v1/webhook":        map[string]any{"post": op("Generic webhook: flash the text, message, title or summary field (or a text/plain body). Optional colour and seconds", jsonBody(obj(map[string]any{"text": str, "colour": str, "seconds": num})))},
			"/v1/webhook/github": map[string]any{"post": map[string]any{"summary": "GitHub webhook (JSON). Flashes failed workflow runs, releases, new issues and PRs, merges and stars. Use ?token=KEY from other machines and &passes=1 to show passing runs", "responses": ok}},
			"/v1/settings": map[string]any{
				"get": op("App settings (local only)", nil),
				"put": op("Change settings (local only): openAtLogin, pagerdutyToken (empty string removes it)", jsonBody(obj(map[string]any{"openAtLogin": boolean, "pagerdutyToken": str}))),
			},
			"/v1/config": map[string]any{
				"get": op("The info screens' settings (local only): {settings: [...]}, each with id, title, help, file, kind (line, lines, secret), example, programs, value (never for a secret), set, current", nil),
			},
			"/v1/config/{id}": map[string]any{
				"put": map[string]any{"summary": "Check and save one setting (local only); an empty value restores the default. 422 with the reason when the value is refused; 200 with a summary of what was understood",
					"parameters":  []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": str}},
					"requestBody": jsonBody(obj(map[string]any{"value": str})), "responses": ok},
			},
			"/v1/widgets": map[string]any{
				"get":  op("The widgets board", nil),
				"post": op("Post one widget or a list: id, label, value, unit, status (ok, warn, bad, info), text, ttl (seconds), flash. The same id replaces it", jsonBody(obj(map[string]any{"id": str, "label": str, "value": str, "unit": str, "status": str, "text": str, "ttl": num, "flash": boolean}, "id"))),
			},
			"/v1/widgets/{id}":    map[string]any{"delete": map[string]any{"summary": "Remove a widget (* for all)", "parameters": []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": str}}, "responses": ok}},
			"/v1/claude/activity": map[string]any{"post": op("Claude Code hook report (local only): session, cwd, event (tool, done, prompt, stop), tool, detail", jsonBody(obj(map[string]any{"session": str, "cwd": str, "event": str, "tool": str, "detail": str}, "event")))},
			"/v1/keys/{id}":       map[string]any{"delete": map[string]any{"summary": "Revoke an API key (local only)", "parameters": []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": str}}, "responses": ok}},
		},
	}
	writeJSON(w, 200, spec)
}
