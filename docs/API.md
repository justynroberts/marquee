# HTTP API

Marquee listens on port 7781. By default it listens on `127.0.0.1` only; with **Allow Other
Devices** on (or `marquee serve --lan`) it listens on every interface and advertises itself over
Bonjour.

```bash
M=http://127.0.0.1:7781
```

All request and response bodies are JSON unless stated otherwise. Errors come back as
`{"error": "message"}` with a 4xx or 5xx status. Every response carries
`Access-Control-Allow-Origin: *`, so browser apps on other origins can call the API.

The running server publishes a machine-readable description at `GET /v1/openapi.json` (OpenAPI
3.1, no key needed). The menu's **API Reference** opens it.

## Authentication

| Request from | Key needed? |
|---|---|
| The Mac running Marquee (loopback) | No, unless **Require Key on This Mac** is on (`--require-key`) |
| Another device | Yes, and **Allow Other Devices** must be on |

Create keys from the menu (**New API Key…**, copied to the clipboard), with
`marquee key create NAME`, or with `POST /v1/keys`. Keys look like `mq_` followed by 48 hex
characters. Marquee stores only a SHA-256 hash, so a key is shown once.

Send the key in any one of these:

| Where | Example |
|---|---|
| `Authorization` header | `Authorization: Bearer mq_…` |
| `X-Marquee-Key` header | `X-Marquee-Key: mq_…` |
| `X-Panel-Token` header | `X-Panel-Token: mq_…` (the Python server's name) |
| `key` query parameter | `/v1/preview.jpg?key=mq_…` (for `<img>` and `EventSource`, which cannot set headers) |
| `token` query parameter | `/v1/webhook/github?token=mq_…` |

A missing or wrong key gets `401` with `WWW-Authenticate: Bearer realm="marquee"`.

**Local-only routes.** `/v1/keys` and `/v1/settings` only answer requests from the Mac running
Marquee, whatever key is sent (`403` otherwise). They also refuse requests whose `Origin` header
is a non-loopback web site, so a page in your browser cannot create keys behind your back.

```bash
# from another machine
curl -H "Authorization: Bearer $MARQUEE_KEY" http://studio.local:7781/v1/state
```

## Discovery

With network access on, Marquee registers `_marquee._tcp` in `local.` as
`Marquee on <hostname>`, with TXT records `version=…`, `api=/v1`, `auth=bearer` and
`docs=/v1/openapi.json`.

```bash
dns-sd -B _marquee._tcp
```

## Routes at a glance

| Method and path | Purpose |
|---|---|
| `GET /v1` | Server, version and panel information |
| `GET /v1/state` | What is playing, overlay, brightness, fps, panel status, timing |
| `GET /v1/programs` | Everything `/v1/play` accepts |
| `POST /v1/play` | Switch program |
| `POST /v1/flash` | Alert over the current program |
| `POST /v1/timer` | Countdown overlay |
| `DELETE /v1/overlay` | Cancel a flash or timer |
| `GET /v1/messages`, `PUT /v1/messages`, `POST /v1/messages` | The message board file |
| `GET /v1/schedule`, `PUT /v1/schedule` | The daily schedule |
| `GET /v1/media`, `POST /v1/media`, `DELETE /v1/media/{name}` | Uploaded pictures and animations |
| `POST /v1/frame` | Show an image now (stream by repeating) |
| `POST /v1/draw` | Set the drawing pad |
| `POST /v1/brightness`, `POST /v1/pause`, `POST /v1/flip` | Display controls |
| `POST /v1/mirror` | Mirror all output |
| `POST /v1/hud` | HUD mode, text, mirror, lyrics |
| `POST /v1/lyrics` | Lyric timing |
| `GET /v1/playlists`, `PUT /v1/playlists/{name}`, `DELETE /v1/playlists/{name}` | Playlists |
| `POST /v1/news/refresh` | Fetch headlines now |
| `POST /v1/webhook` | Generic webhook |
| `POST /v1/webhook/github` | GitHub webhook |
| `GET /v1/preview.jpg` | The frame on the panel now |
| `GET /v1/events` | Server-sent events |
| `GET /v1/fonts` | Font names for the board |
| `GET /v1/update` | Update check against GitHub Releases |
| `GET /v1/keys`, `POST /v1/keys`, `DELETE /v1/keys/{id}` | API keys (local only) |
| `GET /v1/settings`, `PUT /v1/settings` | App settings (local only) |
| `GET /v1/openapi.json` | OpenAPI description (public) |

---

## Information

### GET /v1

```bash
curl $M/v1
```

```json
{
  "name": "Marquee",
  "version": "0.1.0",
  "host": "studio",
  "api": "v1",
  "docs": "/v1/openapi.json",
  "auth": {"localhostNeedsKey": false, "network": false},
  "panel": {
    "status": "connected",
    "width": 1920, "height": 462, "ledCols": 275, "ledRows": 66,
    "model": "Thermalright Trofeo Vision 9.16", "modelId": 65, "deviceId": "0012abcd"
  }
}
```

`panel.status` is `connected`, `disconnected` or `disabled` (`--no-panel`). `model`, `modelId` and
`deviceId` appear only while a panel is connected.

### GET /v1/state

```bash
curl $M/v1/state
```

```json
{
  "program": "show",
  "brightness": 100,
  "paused": false,
  "flip": false,
  "mirror": "horizontal",
  "hudText": "Deep work until 17:00",
  "hudPrev": "show",
  "hudMirror": "vertical",
  "hudLyrics": true,
  "lyricsLead": 1.1,
  "overlay": "flash",
  "fps": 12.1,
  "panel": "connected",
  "panelName": "Thermalright Trofeo Vision 9.16",
  "error": "",
  "timing": {"encodeMs": 21.4, "sendMs": 58.2, "sendMaxMs": 71.0, "frameKB": 612.3, "quality": 88},
  "pagerduty": {"enabled": true, "error": "", "incidents": "1 triggered", "incident": "Checkout API 5xx"}
}
```

| Field | Meaning |
|---|---|
| `program` | Current program name; `push` while showing a pushed frame |
| `brightness` | Software brightness, 5–100 |
| `paused`, `flip` | Display controls |
| `mirror` | Mirror applied to everything (`horizontal`, `vertical`); omitted when off |
| `hudText`, `hudPrev`, `hudMirror`, `hudLyrics` | HUD settings; `hudPrev` is what `hud off` returns to |
| `lyricsLead` | Seconds early that lyric lines appear, once changed from the default |
| `overlay` | `flash` or `timer` while one is showing; omitted otherwise |
| `fps` | Frames per second over the last two seconds |
| `panel`, `panelName` | Panel status and model |
| `error` | The last error (panel, a crashed program, a failed schedule line); omitted when none |
| `timing` | Mean JPEG encode time, mean and worst USB send time (including the panel's acknowledgement), mean frame size and current JPEG quality, over the last two seconds; omitted until frames have been sent |
| `pagerduty` | `enabled`, `error`, and the `incidents` and `incident` token values |

Fields that are empty or false may be omitted.

### GET /v1/programs

```bash
curl $M/v1/programs
```

```json
{"programs": [
  {"name": "audio", "group": "playlist", "description": "mirror, waterfall, scope, pulse, meter, spectrum"},
  {"name": "showcase", "group": "playlist", "description": "A guided tour of everything Marquee can do"},
  {"name": "board", "group": "panel", "description": "Scrolling message board (messages.txt)"},
  {"name": "fire", "group": "scene", "description": "Flames rising from the bottom of the panel"},
  {"name": "spectrum", "group": "audio", "description": "Classic bars with falling peak dots"},
  {"name": "media:party.gif", "group": "media", "description": "party.gif"}
]}
```

Groups are `playlist` (built-in, custom and `showcase`), `panel` (`board`, `draw`, `news`, `hud`),
`scene`, `audio` and `media`.

### GET /v1/fonts

```bash
curl $M/v1/fonts
```

```json
{"shortcuts": ["chalk", "cjk", "condensed", "didot", "futura", "hand", "marker", "mono", "pixel", "poster", "rounded", "sans", "script", "serif", "stencil", "typewriter"],
 "installed": ["Helvetica", "Menlo", "Futura", "..."]}
```

`shortcuts` work as `[name]` board tags (see [BOARD.md](BOARD.md#fonts) for the `cjk` exception);
`installed` names work as `[font=Name]` and `@font=Name`.

---

## Playing

### POST /v1/play

| Field | Type | |
|---|---|---|
| `program` | string, required | A name from `/v1/programs`: a playlist, `showcase`, `board`, `draw`, `news`, `hud`, a scene, an audio visualiser, `nowplaying`, `karaoke`, `dashboard`, `interference`, `bigclock`, `worldclock`, `headlines`, `newsreel`, `newsreel:NAME`, `flights`, `quakes`, `onthisday`, `moon`, `countdown`, `wordclock`, `calendar`, `markets`, `air`, `weather`, `ascii` or `media:FILE` |

```bash
curl -X POST $M/v1/play -H 'Content-Type: application/json' -d '{"program":"matrix"}'
```

```json
{"ok": true, "program": "matrix"}
```

An unknown name gets `404`.

### POST /v1/flash

Shows text over whatever is playing, then returns to it. Colour emoji are supported. In HUD mode
the alert takes the whole screen in amber.

| Field | Type | |
|---|---|---|
| `text` | string, required | Cut to 300 characters |
| `seconds` | number | 1–600, default 10 |
| `colour` (or `color`) | string | A palette name (`white`, `grey`, `red`, `orange`, `yellow`, `green`, `cyan`, `blue`, `purple`, `pink`, `alert`, `black`) or `#rrggbb` / `#rgb`. Default is the vermilion alert colour; an unrecognised value also falls back to it |

```bash
curl -X POST $M/v1/flash -H 'Content-Type: application/json' \
  -d '{"text":"Deploy finished 🚀","seconds":8,"colour":"green"}'
```

```json
{"ok": true}
```

### POST /v1/timer

| Field | Type | |
|---|---|---|
| `minutes` | number | Added to `seconds` |
| `seconds` | number | Added to `minutes` |
| `label` | string | Up to 40 bytes |

With neither `minutes` nor `seconds` the timer runs for five minutes. The total must be more than
zero and no more than 24 hours.

```bash
curl -X POST $M/v1/timer -H 'Content-Type: application/json' -d '{"minutes":5,"label":"standup"}'
```

```json
{"ok": true}
```

### DELETE /v1/overlay

Cancels a flash or timer.

```bash
curl -X DELETE $M/v1/overlay
```

```json
{"ok": true}
```

---

## Message board

The board reads `messages.txt` in the data folder and reloads it within a second of a change. The
markup is in [BOARD.md](BOARD.md).

### GET /v1/messages

```bash
curl $M/v1/messages
```

```json
{"text": "@step=2\n[white]{time}[/]  [cyan]{day}[/] {date}\n..."}
```

If the file does not exist yet, the built-in default messages are returned.

### PUT /v1/messages

Replaces the whole file. `text` is required; up to 200 KB. A trailing newline is added if missing.

```bash
curl -X PUT $M/v1/messages -H 'Content-Type: application/json' \
  -d '{"text":"@step=3\n[green]All systems normal :check:\n[big]{time}"}'
```

```json
{"ok": true, "warnings": null}
```

`warnings` is `null` when the text is clean, or a list of problems the board will tolerate but
you probably did not mean: unknown tags (shown as text), unknown fonts, bad colours, unknown
sprites and bad `@settings`, each with its line number.

### POST /v1/messages

Appends one line. Newlines in `text` become spaces; up to 500 bytes.

```bash
curl -X POST $M/v1/messages -H 'Content-Type: application/json' -d '{"text":"[yellow]Coffee in the kitchen ☕️"}'
```

```json
{"ok": true, "warnings": null}
```

---

## Schedule

`schedule.txt` holds `HH:MM action` lines; format in [GUIDE.md](GUIDE.md#schedule).

### GET /v1/schedule

```bash
curl $M/v1/schedule
```

```json
{"text": "# Daily schedule...\n08:30 dashboard\n23:00 brightness 20\n",
 "entries": [{"at": "08:30", "action": "dashboard"}, {"at": "23:00", "action": "brightness 20"}]}
```

The file is created with commented-out examples if it does not exist.

### PUT /v1/schedule

Replaces the file. `text` is required; up to 50 KB. Returns the lines that parsed.

```bash
curl -X PUT $M/v1/schedule -H 'Content-Type: application/json' \
  -d '{"text":"09:55 timer 5 standup\n17:30 fireworks\n23:00 brightness 20\n"}'
```

```json
{"ok": true, "entries": [{"at": "09:55", "action": "timer 5 standup"}, {"at": "17:30", "action": "fireworks"}, {"at": "23:00", "action": "brightness 20"}]}
```

---

## Media and images

### GET /v1/media

```bash
curl $M/v1/media
```

```json
{"media": ["party.gif", "logo.png"]}
```

### POST /v1/media

Saves a picture or animation to the `media` folder and plays it at once as `media:NAME`. PNG,
JPEG, GIF and WebP are accepted; animated GIF and WebP keep their frame timing (up to 600 frames).
Images are scaled and cropped to fill 1920 x 462. Up to 25 MB.

Either multipart with a field named `file`:

```bash
curl -X POST $M/v1/media -F file=@party.gif
```

or the raw image as the body, with the name in `?name=`:

```bash
curl -X POST "$M/v1/media?name=logo.png" -H 'Content-Type: image/png' --data-binary @logo.png
```

```json
{"ok": true, "program": "media:party.gif"}
```

File names are reduced to letters, digits, `.`, `-` and `_`. A file that is not an image gets
`415`.

### DELETE /v1/media/{name}

```bash
curl -X DELETE $M/v1/media/party.gif
```

```json
{"ok": true}
```

### POST /v1/frame

Shows one image now, without saving it. The body is the image itself (PNG, JPEG, GIF or WebP, up
to 25 MB), any size, scaled and cropped to fill. The program becomes `push` until something else
plays. Post repeatedly to stream from your own app; the panel takes about 12 frames a second.

```bash
curl -X POST $M/v1/frame -H 'Content-Type: image/png' --data-binary @chart.png
```

```json
{"ok": true}
```

### POST /v1/draw

Sets the drawing pad, the 275 x 66 LED canvas the web remote paints on, and plays `draw` if it is
not already playing. Any image size is scaled to the grid (nearest neighbour). Body up to 4 MB.

| Field | Type | |
|---|---|---|
| `png` | string, required | A PNG data URL (`data:image/png;base64,...`) or bare base64 |

```bash
curl -X POST $M/v1/draw -H 'Content-Type: application/json' \
  -d "{\"png\":\"data:image/png;base64,$(base64 -i sketch.png)\"}"
```

```json
{"ok": true}
```

### GET /v1/preview.jpg

The frame on the panel now, as JPEG, after brightness and mirroring. Not cached.

```bash
curl -o now.jpg $M/v1/preview.jpg
```

---

## Display controls

### POST /v1/brightness

| Field | Type | |
|---|---|---|
| `percent` | integer | Clamped to 5–100. Software brightness: pixel values are scaled before sending |

```bash
curl -X POST $M/v1/brightness -H 'Content-Type: application/json' -d '{"percent":40}'
```

```json
{"ok": true, "percent": 40}
```

### POST /v1/pause

| Field | Type | |
|---|---|---|
| `paused` | boolean | Default `true`. While paused no frames are sent and the panel holds its last image |

```bash
curl -X POST $M/v1/pause -H 'Content-Type: application/json' -d '{"paused":false}'
```

```json
{"ok": true, "paused": false}
```

### POST /v1/flip

Turns the image upside down, for a panel mounted the other way up.

| Field | Type | |
|---|---|---|
| `flip` | boolean | Default `false` |

```bash
curl -X POST $M/v1/flip -H 'Content-Type: application/json' -d '{"flip":true}'
```

```json
{"ok": true, "flip": true}
```

### POST /v1/mirror

Mirrors everything sent to the panel, whatever is playing. When set, it also overrides the HUD's
own mirror.

| Field | Type | |
|---|---|---|
| `mirror` | string | `horizontal` (or `h`), `vertical` (or `v`), `off` (or `none`, or empty) |

```bash
curl -X POST $M/v1/mirror -H 'Content-Type: application/json' -d '{"mirror":"off"}'
```

```json
{"ok": true, "mirror": ""}
```

---

## HUD and lyrics

### POST /v1/hud

Any combination of fields; send `{}` to read the current settings.

| Field | Type | |
|---|---|---|
| `on` | boolean | `true` plays the HUD (remembering the current program); `false` returns to that program |
| `text` | string | The HUD's message line, up to 120 characters; `""` clears it |
| `mirror` | string | How the HUD is mirrored: `horizontal`, `vertical` or `off` |
| `lyrics` | boolean | Show synced lyrics instead of the clock while the song has them. This does not turn the HUD on by itself |

```bash
curl -X POST $M/v1/hud -H 'Content-Type: application/json' \
  -d '{"on":true,"text":"Deep work until 17:00","mirror":"vertical"}'
```

```json
{"ok": true, "on": true, "text": "Deep work until 17:00", "mirror": "vertical", "lyrics": false}
```

### POST /v1/lyrics

How many seconds early lyric lines appear, for `karaoke` and HUD lyrics. Default 1.1, range −5 to
5. Kept across restarts.

| Field | Type | |
|---|---|---|
| `lead` | number | Set the lead |
| `adjust` | number | Add to the lead: positive makes lines appear earlier, negative later |

Send `{}` to read the current value (an empty body is rejected as invalid JSON).

```bash
curl -X POST $M/v1/lyrics -H 'Content-Type: application/json' -d '{"adjust":0.25}'
```

```json
{"ok": true, "lead": 1.5}
```

---

## Playlists

### GET /v1/playlists

Built-in and custom playlists (not `showcase`, which is a program of its own).

```bash
curl $M/v1/playlists
```

```json
{
  "show":  {"items": ["matrix", "terminal", "board", "..."], "builtin": true},
  "green": {"items": ["matrix", "terminal", "board"], "builtin": true},
  "audio": {"items": ["mirror", "waterfall", "scope", "pulse", "meter", "spectrum"], "builtin": true},
  "desk":  {"items": ["nowplaying", "news", "clock", "fire"], "seconds": 12}
}
```

### PUT /v1/playlists/{name}

Creates or replaces a custom playlist, saved in `playlists.json`.

| Field | Type | |
|---|---|---|
| `items` | array of strings, required | 1–100 program names: scenes, audio visualisers, `board`, `news`, `nowplaying`, `karaoke`, `dashboard`, `interference`, `bigclock`, `worldclock`, `headlines`, `newsreel`, `newsreel:NAME`, `flights`, `quakes`, `onthisday`, `moon`, `countdown`, `wordclock`, `calendar`, `markets`, `air`, `weather`, `ascii`, `hud`, `media:FILE` |
| `seconds` | number | Seconds per item, 0–3600; 0 or missing uses the default (20) |

`board` and `news` items run for one full scroll rather than `seconds`. Names are lowercase
letters, digits, `-` and `_`, up to 40 characters, and may not be a built-in playlist or an
existing program name. Built-in playlists cannot be items. Every item is checked before the
playlist is saved; a bad one gets `400` naming it.

```bash
curl -X PUT $M/v1/playlists/desk -H 'Content-Type: application/json' \
  -d '{"items":["nowplaying","news","clock","fire"],"seconds":12}'
curl -X POST $M/v1/play -H 'Content-Type: application/json' -d '{"program":"desk"}'
```

```json
{"ok": true, "name": "desk"}
```

### DELETE /v1/playlists/{name}

```bash
curl -X DELETE $M/v1/playlists/desk
```

```json
{"ok": true}
```

Built-in playlists cannot be deleted (`404`).

---

## News

### POST /v1/news/refresh

Fetches headlines now for the `news` ticker (it also refreshes every ten minutes while used). No
body.

```bash
curl -X POST $M/v1/news/refresh
```

```json
{"ok": true}
```

`502` if no source could be fetched.

---

## Webhooks

### POST /v1/webhook

Flashes text from any service that can send a webhook. The text is taken from the first non-empty
`text`, `message`, `title`, `summary` or `content` field of a JSON body, or from a `text/*` body.
Whitespace is collapsed and the text is cut to 300 characters.

| Field | Type | |
|---|---|---|
| `text` (or `message`, `title`, `summary`, `content`) | string, required | What to flash |
| `colour` (or `color`) | string | Palette name or `#rrggbb`; an unknown colour gets `400` |
| `seconds` | number | 1–600, default 10 |

```bash
curl -X POST $M/v1/webhook -H 'Content-Type: application/json' -d '{"text":"Deploy done","colour":"green"}'
curl -X POST $M/v1/webhook -H 'Content-Type: text/plain' --data 'Backup finished'
```

```json
{"ok": true}
```

From another machine, put the key in the URL if the sender cannot set headers:
`http://studio.local:7781/v1/webhook?token=mq_…`.

### POST /v1/webhook/github

Turns GitHub webhook deliveries into flashes, based on the `X-GitHub-Event` header:

| Event | Shown when | Flash | Colour, seconds |
|---|---|---|---|
| `ping` | the hook is created | `GitHub connected · REPO ✅` | green, 6 |
| `workflow_run` | completed with `failure`, `timed_out` or `startup_failure` | `🔥 NAME failed · REPO · BRANCH` | vermilion, 30 |
| `workflow_run` | completed with `success`, only with `&passes=1` | `✅ NAME passed · REPO · BRANCH` | green, 6 |
| `release` | published | `🚀 REPO TAG released` | green, 15 |
| `issues` | opened | `Issue #N · REPO · TITLE` | amber, 12 |
| `pull_request` | opened | `PR #N · REPO · TITLE` | amber, 12 |
| `pull_request` | closed and merged | `Merged #N · REPO · TITLE` | green, 10 |
| `star` | created | `⭐ USER starred REPO` | amber, 8 |

Anything else is accepted and ignored. The response says whether something was shown:

```json
{"ok": true, "shown": true, "text": "🔥 CI failed · marquee · main"}
```

```json
{"ok": true, "shown": false}
```

**Setting it up.** GitHub calls from the internet, so the Mac running Marquee must be reachable
from it: turn on **Allow Other Devices** and expose port 7781 through a tunnel or port forward you
control. Then, in the repository (or organisation) on GitHub:

1. **Settings → Webhooks → Add webhook**.
2. **Payload URL**: `https://YOUR-HOST/v1/webhook/github?token=mq_…` (create a key for GitHub
   with `marquee key create github`). Add `&passes=1` to see passing workflow runs too.
3. **Content type**: `application/json`. Marquee does not read form-encoded payloads.
4. **Secret**: leave empty; Marquee authenticates with the key in the URL and does not check
   GitHub's signature.
5. **Which events**: choose **Let me select individual events** and tick **Workflow runs**,
   **Releases**, **Issues**, **Pull requests** and **Stars** (or any subset).
6. **Add webhook**. GitHub sends a `ping`, which flashes `GitHub connected`.

---

## Event stream

### GET /v1/events

Server-sent events. On connect you get a `state` event with the full state, then events as things
change. A `: keepalive` comment is sent every 15 seconds.

| Event | Data |
|---|---|
| `state` | The same object as `GET /v1/state` without `pagerduty`; sent on program, brightness, pause, flip, mirror and HUD changes |
| `overlay` | `{"overlay": "flash"}`, `{"overlay": "timer"}` or `{"overlay": ""}` when it ends |
| `panel` | The `panel` object from `GET /v1`, when the panel connects or disconnects |
| `error` | `{"error": "message"}` when a new error appears |

```bash
curl -N $M/v1/events
```

```
event: state
data: {"program":"show","brightness":100,"paused":false,"flip":false,"fps":12.1,"panel":"connected",...}

event: overlay
data: {"overlay":"flash"}
```

In a browser from another device, pass the key as `?key=`:

```js
const es = new EventSource("http://studio.local:7781/v1/events?key=mq_…");
es.addEventListener("state", e => console.log(JSON.parse(e.data).program));
```

Slow consumers may miss events; re-read `/v1/state` if you need certainty.

---

## Updates

### GET /v1/update

Reports the last check of GitHub Releases for a newer Marquee; `?check=1` checks now. This route
is new; see `GET /v1/openapi.json` for its current response shape.

```bash
curl "$M/v1/update?check=1"
```

---

## API keys (this Mac only)

### GET /v1/keys

```bash
curl $M/v1/keys
```

```json
{"keys": [{"id": "1a2b3c4d", "name": "github", "prefix": "mq_9f3e0a", "hash": "",
           "created": "2026-11-07T09:12:00Z", "lastUsed": "2026-11-07T10:40:12Z"}]}
```

`lastUsed` is omitted for keys never used. `hash` is always empty here.

### POST /v1/keys

| Field | Type | |
|---|---|---|
| `name` | string | What will use the key; `unnamed` if empty |

```bash
curl -X POST $M/v1/keys -H 'Content-Type: application/json' -d '{"name":"stream deck"}'
```

`201`:

```json
{"key": {"id": "5e6f7a8b", "name": "stream deck", "prefix": "mq_41c2d0", "hash": "…", "created": "…"},
 "secret": "mq_41c2d0...",
 "note": "Store the secret now; it is not shown again."}
```

### DELETE /v1/keys/{id}

```bash
curl -X DELETE $M/v1/keys/5e6f7a8b
```

```json
{"ok": true}
```

---

## Settings (this Mac only)

### GET /v1/settings

```bash
curl $M/v1/settings
```

```json
{"lan": false, "requireKey": false, "openAtLogin": true,
 "pagerduty": {"enabled": false, "error": "", "incidents": "PagerDuty not set up", "incident": ""}}
```

### PUT /v1/settings

Any combination of fields. Returns the settings as `GET` does.

| Field | Type | |
|---|---|---|
| `pagerdutyToken` | string | Saves the PagerDuty API token to `pagerduty-token` (mode 0600) and starts polling; `""` removes it |
| `openAtLogin` | boolean | Adds or removes a LaunchAgent that opens Marquee at login. Only works when running from `Marquee.app`; otherwise `500` with the reason |

```bash
curl -X PUT $M/v1/settings -H 'Content-Type: application/json' -d '{"pagerdutyToken":"u+abc123..."}'
```

Network access and Require Key are changed from the menu, or with `serve --lan` and
`--require-key`.

---

## Python-compatible routes

These keep scripts written for the earlier Python panel server working. They take the same
authentication as `/v1`.

| Route | Same as |
|---|---|
| `GET /api/state` | Python-shaped state: `program`, `flash`, `overlay`, `fps`, `brightness`, `paused`, `error`, `programs`, `media`, `pagerduty` |
| `GET /api/preview.jpg` | `GET /v1/preview.jpg` |
| `POST /api/play` | `POST /v1/play` |
| `POST /api/flash` | `POST /v1/flash` |
| `POST /api/say` | `POST /v1/messages` |
| `GET /api/messages`, `PUT /api/messages` | `/v1/messages` |
| `POST /api/media` | `POST /v1/media` |
| `POST /api/brightness` | `POST /v1/brightness` |
| `POST /api/timer` | `POST /v1/timer` |
| `DELETE /api/overlay` | `DELETE /v1/overlay` |
| `POST /api/draw` | `POST /v1/draw` |
| `GET /api/schedule`, `PUT /api/schedule` | `/v1/schedule` |
| `POST /api/pause` | `POST /v1/pause` |

New code should use `/v1`.

## OpenAPI

`GET /v1/openapi.json` returns a hand-written OpenAPI 3.1 document for the `/v1` routes. It needs
no key, so tools can fetch it from another machine before they have one.

```bash
curl -s $M/v1/openapi.json | jq '.paths | keys'
```
