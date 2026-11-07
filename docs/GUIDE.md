# Marquee guide

How each part of Marquee works and how to set it up. For the HTTP routes see [API.md](API.md); for
the message board markup see [BOARD.md](BOARD.md).

- [Programs](#programs)
- [Playlists](#playlists)
- [Showcase](#showcase)
- [Now Playing and karaoke](#now-playing-and-karaoke)
- [HUD mode](#hud-mode)
- [News ticker](#news-ticker)
- [Alerts and timers](#alerts-and-timers)
- [PagerDuty](#pagerduty)
- [Pictures, animations and your own frames](#pictures-animations-and-your-own-frames)
- [Display controls](#display-controls)
- [Schedule](#schedule)
- [Data folder](#data-folder)
- [Network access and API keys](#network-access-and-api-keys)
- [Running without the menu bar](#running-without-the-menu-bar)
- [MCP: Claude Code and Claude Desktop](#mcp-claude-code-and-claude-desktop)
- [Troubleshooting](#troubleshooting)

## Programs

Exactly one program plays at a time. Play one from the menu (**Play**), the web remote,
`marquee play NAME`, `POST /v1/play`, a playlist or the schedule. Marquee remembers the program,
brightness and other display settings across restarts. `GET /v1/programs` lists everything.

**Scenes** are drawn on a 275 x 66 grid of round LEDs:

| Name | |
|---|---|
| `matrix` | Digital rain: mirrored katakana falling in phosphor green |
| `terminal` | A green-phosphor terminal typing live system information |
| `fire` | Flames rising from the bottom of the panel |
| `life` | Conway's Game of Life, cells coloured by age, reseeds when it settles |
| `warp` | Flying through a starfield |
| `plasma` | A flowing colour field |
| `rain` | Falling streams of light with bright heads |
| `clock` | A big clock with the date stacked left and a seconds sweep |
| `bounce` | An emoji bouncing around the panel, changing on every wall hit |
| `fireworks` | Rockets that burst into coloured sparks |
| `aquarium` | Emoji fish, rising bubbles and swaying weed |

**Full-resolution screens** are drawn at the panel's native 1920 x 462, so small text stays sharp:

| Name | |
|---|---|
| `dashboard` | Clock, CPU (big percentage, two-minute trace, per-core bars), memory, network and machine details |
| `interference` | Three drifting wave sources drawn as contour bands, with a clock and fps label |
| `nowplaying` | The track playing in Music or Spotify |
| `karaoke` | Synced lyrics for that track |

**Camera scenes** draw the webcam, mirrored like a selfie view (see [Troubleshooting](#camera)
if they show a message instead):

| Name | |
|---|---|
| `camera` | The webcam as a dot matrix, auto-levelled; the panel's shape crops it to a band across the middle |
| `popart` | Three posterised panels of the whole picture, in bold palettes that change every four seconds |
| `motion` | Only what moves lights up, leaving trails that drift through the colours; a still room is dark |
| `slitscan` | Time displacement: the centre column is live, and each column out is a frame older (about 11 s at the edges) |

**Audio visualisers** listen to the microphone (see [Troubleshooting](#microphone) if they show a
message instead):

| Name | |
|---|---|
| `spectrum` | Classic bars with falling peak dots |
| `mirror` | Symmetric bars from the centre, rainbow by frequency; beats flash the background |
| `waterfall` | Scrolling spectrogram: time runs right to left, bass at the bottom |
| `scope` | Triggered oscilloscope with phosphor afterglow |
| `pulse` | Every beat fires a ring and sparks; bass glows the core; treble twinkles |
| `meter` | Segmented LED graphic equaliser with peak hold |

**Panels**: `board` (the message board), `news` (the headline ticker), `hud` (the heads-up
display) and `draw` (the drawing pad the web remote paints on).

**Media**: every uploaded file plays as `media:FILENAME`.

## Playlists

A playlist plays programs in turn and loops. Three are built in:

| Name | Items |
|---|---|
| `show` | matrix, terminal, board, fireworks, fire, life, warp, board, aquarium, plasma, rain, clock, bounce, board |
| `green` | matrix, terminal, board |
| `audio` | mirror, waterfall, scope, pulse, meter, spectrum |

Each item plays for 20 seconds (`marquee serve --scene-seconds N` changes this), except `board`
and `news`, which play for one full scroll of their text.

Make your own with `PUT /v1/playlists/NAME` or the MCP `set_playlist` tool:

```bash
curl -X PUT http://127.0.0.1:7781/v1/playlists/desk -H 'Content-Type: application/json' \
  -d '{"items":["nowplaying","news","clock","fire","media:logo.png"],"seconds":12}'
marquee play desk
```

- Items are any program names: scenes, visualisers, `board`, `news`, `nowplaying`, `karaoke`,
  `dashboard`, `interference`, `hud` or `media:FILE`. Built-in playlists cannot be items.
- `seconds` is per item, up to 3600; leave it out for the default.
- Names are lowercase letters, digits, `-` and `_`, up to 40 characters, and cannot clash with an
  existing program.
- Every item is checked when you save; a typo is rejected with the item named.

Custom playlists are saved in `playlists.json` and appear in the menu under **Play → Playlists**
after the next launch. Delete one with `DELETE /v1/playlists/NAME`.

## Showcase

`showcase` is a guided tour: 25 parts, each introduced by a title card with its number, name, a
one-line caption and a progress rule. It covers the message board (colours, effects, fonts,
sprites, emoji, tokens and an alert line), the green phosphor theme, the scenes, Now Playing,
the news ticker, the dashboard, interference, every audio visualiser, a flash and a timer. It then
starts again. Play some music before the audio section.

The showcase writes its own board text to `showcase-board.txt` and `showcase-green.txt` in the data
folder; your `messages.txt` is not touched.

## Now Playing and karaoke

Both read Apple Music and Spotify once a second through AppleScript. A player that is not running
is never launched. A playing track is preferred to a paused one, and Music to Spotify when both
are in the same state. The first time, macOS asks
whether Marquee may control Music or Spotify (Automation); allow it.

**`nowplaying`** shows square artwork, a small source line, the title large (scrolling if it is too
long), artist, album, and a progress rule with elapsed and remaining time. It never shows lyrics.
With nothing playing it says *Play something in Music or Spotify*.

**`karaoke`** shows the current lyric line large, sweeping from dim to bright as it is sung, with
the next line beneath. Lyrics come from [LRCLIB](https://lrclib.net), a free, open lyrics database
that needs no account. Marquee sends only the track's title, artist, album and length, once per
track. If LRCLIB has no time-synced lyrics for the song, `karaoke` falls back to the Now Playing
screen.

### Lyric timing

Each line is shown slightly before its timestamp, 1.1 seconds by default, to cover the time it
takes to ask the player and get a frame onto the panel. If lines appear late (common with
Bluetooth or AirPlay speakers) or early, nudge the lead:

```bash
marquee lyrics            # print the current lead
marquee lyrics earlier    # lines appear 0.25 s earlier
marquee lyrics later      # lines appear 0.25 s later
marquee lyrics 2          # set it: lines appear 2 s early
```

In the menu, **Lyrics Timing** shows the current lead and has Earlier and Later (0.1 s steps) and
Reset. The range is −5 to 5 seconds. The value is saved and applies to `karaoke` and HUD lyrics.
Over HTTP: `POST /v1/lyrics` with `{"adjust":0.25}` or `{"lead":2}`.

## HUD mode

HUD mode is a plain heads-up screen meant to be read **reflected in your monitor**: pure black
(which vanishes in the reflection), bold white type, a huge clock and one line beside it. The
line is, in order of preference, your HUD text, the track playing in Music or Spotify, or the day.

Turn it on from the menu (**HUD Mode**), with `marquee hud on`, or `POST /v1/hud {"on":true}`.
Turning it off returns to whatever was playing before.

```bash
marquee hud on
marquee hud Deep work until 17:00      # set the line
marquee hud clear                      # clear it (falls back to the track or the day)
marquee hud off
```

The text is up to 120 characters.

### Mirror: horizontal or vertical

A reflection reverses the image, so the HUD is drawn reversed to read correctly in the glass.
Which way depends on where the panel sits:

| Mirror | Panel position | What the reflection does |
|---|---|---|
| `horizontal` (default) | Standing in front of the screen, facing it, e.g. on the desk below your eyeline | Reverses left and right |
| `vertical` | Lying flat below the screen, face up | Turns top to bottom |
| `off` | You look at the panel directly | Nothing |

```bash
marquee hud mirror vertical
```

The HUD mirror applies only while the HUD plays. There is also a global mirror for every program
(`POST /v1/mirror`); when that is set, it takes precedence over the HUD's own. **Upside Down** in
the menu is something else again: a 180° rotation for a panel mounted the other way up.

### HUD lyrics

With **HUD Lyrics** on (menu, `marquee hud lyrics on`, or `{"lyrics":true}`), the HUD shows the
current lyric line, sweeping from grey to white, with the next line beneath, in place of the clock.
When the song has no synced lyrics, or nothing is playing, the clock comes back. Choosing HUD Lyrics
in the menu also turns the HUD on; the command and API only change the setting.

### Alerts in HUD mode

A flash (from the API, a webhook, PagerDuty or the schedule) takes the whole HUD in amber, mirrored
like the rest. Timers keep their normal countdown look.

## News ticker

`news` scrolls headlines across the panel: the source in orange, the title in white and, for
Hacker News, the points. It fetches the top 15 Hacker News stories when first played and every ten
minutes after that.

To use other sources, create `news-sources.txt` in the data folder with one source per line:

```
# news-sources.txt
hn
https://feeds.bbci.co.uk/news/technology/rss.xml
https://www.theverge.com/rss/index.xml
```

- `hn` (or `hackernews`) is Hacker News. Leave it out to drop it.
- Any `http://` or `https://` URL of an RSS 2.0 or Atom feed; the first ten items of each are used,
  labelled with the feed's own title.
- Blank lines and `#` comments are ignored. With no valid lines, Hacker News is used.

Fetch now with `POST /v1/news/refresh` (or the MCP `refresh_news` tool). The ticker is the message
board reading `news.txt`, which Marquee rewrites; editing `news.txt` yourself is pointless. If every
source fails, the ticker says the headlines are unavailable.

## Alerts and timers

A **flash** shows text over whatever is playing, then goes back to it:

```bash
marquee flash Build failed 🔥 --seconds 20 --colour red
```

Text is cut to 300 characters; seconds are 1–600 (default 10); the colour is a palette name or
`#rrggbb` (default vermilion). Text that fits is shown large and still; longer text glides across.

A **timer** counts down over the current program:

```bash
marquee timer 5 standup
```

**Clear Alert** in the menu, `DELETE /v1/overlay` or the MCP `clear_alert` tool cancels either.
A new flash or timer replaces the one showing.

Webhooks turn other services' events into flashes: see
[API.md](API.md#webhooks) for the generic hook and GitHub.

## PagerDuty

Marquee can watch PagerDuty for open incidents. It needs a PagerDuty REST API key; a read-only key
is enough (PagerDuty → Integrations → API Access Keys).

Set it in one of these ways:

- From the Mac running Marquee:
  ```bash
  curl -X PUT http://127.0.0.1:7781/v1/settings -H 'Content-Type: application/json' \
    -d '{"pagerdutyToken":"YOUR_KEY"}'
  ```
  This writes `pagerduty-token` in the data folder (readable only by you) and starts polling at
  once. Send `""` to remove it.
- Put the key in `~/Library/Application Support/Marquee/pagerduty-token` yourself and restart
  Marquee.
- For `marquee serve` started from a shell, set `PAGERDUTY_TOKEN`; it takes precedence over the
  file.

Two more environment variables apply to `marquee serve`:

| Variable | |
|---|---|
| `PAGERDUTY_API` | API base for EU accounts: `https://api.eu.pagerduty.com` |
| `PAGERDUTY_SERVICE_IDS` | Comma-separated service IDs to watch; all services otherwise |

Marquee polls every 60 seconds for triggered and acknowledged incidents. A newly triggered
incident flashes for 30 seconds as `INCIDENT: title`, or `P1 INCIDENT: title` for high urgency.
Incidents already open when Marquee starts do not flash. On the message board, `{incidents}`
shows a summary such as `2 triggered · 1 acknowledged` and `{incident}` the newest title.
`GET /v1/state` shows the feed's state under `pagerduty`.

## Pictures, animations and your own frames

- **Upload**: drop a PNG, JPEG, GIF or WebP onto the web remote, or `POST /v1/media`. It is saved in
  `media/` and plays straight away as `media:FILENAME`. Animated GIF and WebP keep their own frame
  timing (frames shorter than 50 ms are slowed to 50 ms; up to 600 frames). Pictures are scaled and
  cropped to fill the 1920 x 462 panel, so a 4:1 image fits best.
- **Show once**: `marquee send FILE` or `POST /v1/frame` shows an image without saving it.
- **Stream**: post frames to `/v1/frame` repeatedly from your own program. The panel takes about
  12 a second; anything beyond that is wasted.
- **Draw**: the web remote's drawing pad paints straight onto the LED grid (`draw`).

## Display controls

| Control | Menu | Command / API |
|---|---|---|
| Brightness, 5–100 % | Brightness | `POST /v1/brightness`, schedule `brightness N` |
| Pause: stop sending frames | Pause | `POST /v1/pause`, schedule `pause` / `resume` |
| Upside down | Upside Down | `POST /v1/flip` |
| Mirror everything | | `POST /v1/mirror` |

Brightness is applied in software, by scaling the image before it is sent.

## Schedule

`schedule.txt` in the data folder runs actions at set times every day. Edit it in the web remote,
a text editor, or with `PUT /v1/schedule`. It is re-read every minute, so changes apply without a
restart. If it is missing, Marquee creates one with every example commented out.

One action per line: a 24-hour time `HH:MM` (two digits each), a space, then the action.

| Action | |
|---|---|
| a program name | Play it: `show`, `board`, `dashboard`, `hud`, `media:logo.png`, a custom playlist… |
| `brightness N` | Set brightness, 5–100 |
| `pause` | Stop sending frames |
| `resume` | Start again |
| `flash TEXT` | Flash TEXT for 15 seconds in the alert colour |
| `timer MINUTES LABEL` | Start a countdown; the label is optional |

```
# schedule.txt
08:30 dashboard
09:00 brightness 100
09:55 timer 5 standup
12:30 flash Lunch!
17:30 fireworks
18:00 green
23:00 brightness 20
23:30 pause
07:30 resume
```

Lines starting with `#`, blank lines and lines that do not parse (`9:00`, `25:00`, no action) are
ignored. `GET /v1/schedule` returns the lines that parsed. An action that fails at its time, such
as an unknown program, shows in `GET /v1/state` as `error`. Actions run once, at the start of their
minute, while Marquee is running; missed times are not caught up.

## Data folder

Everything Marquee keeps is in `~/Library/Application Support/Marquee` (menu → **Show Data
Folder**). On Linux it is `~/.config/marquee`; `marquee serve --data DIR` uses another folder.

| File | |
|---|---|
| `messages.txt` | The message board. Yours to edit |
| `schedule.txt` | The daily schedule. Yours to edit |
| `news-sources.txt` | News ticker sources. Yours to create |
| `pagerduty-token` | PagerDuty API key |
| `playlists.json` | Custom playlists |
| `media/` | Uploaded pictures and animations |
| `keys.json` | API keys, as SHA-256 hashes only (mode 0600) |
| `settings.json` | Allow Other Devices and Require Key |
| `state.json` | Program, brightness, pause, flip, mirror, HUD and lyric settings |
| `news.txt` | Current headlines, written by Marquee |
| `showcase-board.txt`, `showcase-green.txt` | Written by the showcase |

Other places:

| Path | |
|---|---|
| `~/Library/Logs/Marquee/marquee.log` | Log of the menu-bar app (`marquee serve` logs to the terminal) |
| `~/Library/LaunchAgents/com.fintonlabs.marquee.plist` | Present while **Open at Login** is on |

To start afresh, quit Marquee and move the data folder away.

## Network access and API keys

By default Marquee listens only on `127.0.0.1:7781`, so only apps on the same Mac can reach it,
and they need no key.

To control the panel from other devices:

1. Turn on **Allow Other Devices** in the menu. Marquee now listens on every interface and
   advertises itself over Bonjour. macOS may ask to allow local network access; allow it.
2. Create a key: **New API Key…** (named after the device or app; the key is copied to the
   clipboard and shown once), or `marquee key create NAME` on the Mac running Marquee.
3. On the other device, send it as `Authorization: Bearer mq_…`.
   **Copy Network Address** in the menu copies `http://<this Mac's IP>:7781`.

```bash
export MARQUEE_URL=http://studio.local:7781 MARQUEE_KEY=mq_…
marquee status
```

The web remote works from other devices too: open `http://studio.local:7781/?key=mq_…`.

- **Require Key on This Mac** makes apps on the same Mac need a key too.
- Keys and settings can only be changed from the Mac running Marquee, never over the network.
- `marquee key list` shows keys with when they were last used; `marquee key revoke ID` deletes one.

### Bonjour

With network access on, Marquee advertises `_marquee._tcp` as *Marquee on HOSTNAME*, with TXT
records `version`, `api=/v1`, `auth=bearer` and `docs=/v1/openapi.json`. Browse for it with:

```bash
dns-sd -B _marquee._tcp
```

## Running without the menu bar

`marquee serve` runs the same server, web remote and API in a terminal. It is the default when the
binary is run outside `Marquee.app`, and it works on Linux with libusb.

| Flag | |
|---|---|
| `--addr ADDR` | Listen address; default `127.0.0.1:7781`, or `:7781` with `--lan` |
| `--lan` | Listen on the network (other devices need a key) and advertise over Bonjour |
| `--require-key` | Require a key from this machine too |
| `--no-panel` | Do not touch USB; render and preview only |
| `--data DIR` | Data folder |
| `--scene-seconds N` | Seconds per item in playlists (default 20) |

`--lan` and `--require-key` override what the menu-bar app saved; without them the saved settings
apply. Only one Marquee can own the panel and port 7781 at a time, so quit the menu-bar app first.

The direct commands talk to the panel without a server, for diagnosis. They fail while Marquee is
running, because it owns the panel.

```bash
marquee info              # model, size and device ID
marquee colour ff5b38     # fill the panel with one colour
marquee image photo.jpg   # show an image
marquee bench             # send 60 frames and report frames per second
```

## MCP: Claude Code and Claude Desktop

`marquee mcp` is a Model Context Protocol server on stdin and stdout. It does not touch USB; it
calls the running Marquee's HTTP API, using `MARQUEE_URL` (default `http://127.0.0.1:7781`) and
`MARQUEE_KEY`. Marquee must be running.

**Claude Code**, panel on this Mac:

```bash
claude mcp add marquee -- /Applications/Marquee.app/Contents/MacOS/marquee mcp
```

Panel on another Mac (turn on Allow Other Devices there and run `marquee key create claude` on it):

```bash
claude mcp add marquee -e MARQUEE_URL=http://studio.local:7781 -e MARQUEE_KEY=mq_… \
  -- /Applications/Marquee.app/Contents/MacOS/marquee mcp
```

**Claude Desktop**: add to `~/Library/Application Support/Claude/claude_desktop_config.json` and
restart Claude Desktop:

```json
{
  "mcpServers": {
    "marquee": {
      "command": "/Applications/Marquee.app/Contents/MacOS/marquee",
      "args": ["mcp"],
      "env": { "MARQUEE_URL": "http://127.0.0.1:7781" }
    }
  }
}
```

Add `"MARQUEE_KEY": "mq_…"` to `env` for a panel on another machine.

Tools:

| Tool | |
|---|---|
| `panel_status` | What is playing, brightness, overlay, fps, panel connection |
| `list_programs` | Everything that can play, grouped |
| `play` | Play a program by name |
| `flash` | Alert over the current program |
| `timer` | Countdown |
| `clear_alert` | Cancel a flash or timer |
| `say` | Append a board message |
| `get_messages`, `set_messages` | Read or replace `messages.txt` |
| `list_fonts` | Board font names |
| `list_playlists`, `set_playlist`, `delete_playlist` | Custom playlists |
| `refresh_news` | Fetch headlines now |
| `hud` | HUD on or off, text, mirror, lyrics |
| `brightness`, `pause`, `flip` | Display controls |
| `show_image` | Show a local file or a URL (up to 10 MB) |
| `preview` | A JPEG of what the panel shows now, so Claude can see it |

The `say` and `set_messages` tools carry a summary of the board markup, so Claude can write styled
messages without reading [BOARD.md](BOARD.md).

## Troubleshooting

### Panel not found

The menu's first line says *Panel not found*, and `GET /v1/state` shows `"panel": "disconnected"`
with an `error` such as `panel: usb: device not found`. Marquee retries every two seconds.

- Check the panel's USB cable is in a data port of the Mac (not a charge-only hub) and that the
  panel has power. `system_profiler SPUSBDataType | grep -A3 -i 0x5408` should list it.
- Quit anything else that talks to the panel: Thermalright's own software, another copy of
  Marquee, or a `marquee serve` in a terminal. A busy panel gives an error like
  `usb claim interface: LIBUSB_ERROR_ACCESS` or `LIBUSB_ERROR_BUSY`.
- A panel left mid-conversation by another program is reset automatically; if it still will not
  answer (`handshake read` errors), unplug it for a few seconds.
- On Linux, your user needs permission for the device (a udev rule for `0416:5408`).

### Microphone

The audio visualisers draw a message instead of bars when they cannot hear anything:

- **AUDIO / no microphone: …**: capture could not start, usually because there is no input device.
  Connect or select one in System Settings → Sound → Input. Marquee retries every five seconds.
- **MICROPHONE ACCESS NEEDED**: the input delivers only silence, which is what macOS does when
  permission is denied. Turn on Marquee in System Settings → Privacy & Security → Microphone and
  play the visualiser again. For `marquee serve` in a terminal, the terminal app is what needs the
  permission. To be asked again: `tccutil reset Microphone com.fintonlabs.marquee`.

Quiet music in a loud room, or a muted input, also looks flat: check the input level in Sound
settings.

### Camera

The camera scenes draw a message instead of the picture when there is none:

- **CAMERA / starting...**: the camera takes a second or so to open.
- **CAMERA / Allow camera access in the dialog**: macOS is asking for permission.
- **CAMERA ACCESS NEEDED**: permission was refused. Turn on Marquee in System Settings → Privacy &
  Security → Camera. Marquee retries every three seconds. For `marquee serve` in a terminal, the
  terminal app is what needs the permission. To be asked again:
  `tccutil reset Camera com.fintonlabs.marquee`.
- **CAMERA / no camera**: no camera is connected, or another app has it exclusively.

### Now Playing shows "Allow Marquee to control Music"

That is AppleScript error −1743: macOS has not allowed Marquee to send Automation events to Music
or Spotify. Open System Settings → Privacy & Security → Automation, find Marquee, and turn on Music
and Spotify. If Marquee is not listed, reset the decision so macOS asks again, then play
`nowplaying`:

```bash
tccutil reset AppleEvents com.fintonlabs.marquee
```

If `karaoke` shows the plain Now Playing screen, LRCLIB has no synced lyrics for that track.

### Slow or jerky frames

`GET /v1/state` reports `fps` and a `timing` block averaged over the last two seconds:

| Field | Healthy | |
|---|---|---|
| `fps` | about 12 | The panel's maximum is about 12 frames a second |
| `encodeMs` | well under 80 | JPEG encoding; it overlaps the USB send |
| `sendMs`, `sendMaxMs` | about 80 or less | USB transfer including the panel's acknowledgement |
| `frameKB` | under 1024 | Frames above 1 MB make Marquee lower JPEG quality |
| `quality` | 88 | Drops towards 70 for very busy frames and recovers by itself |

Some programs deliberately draw less often: the dashboard twice a second, the HUD four times a
second (every frame with lyrics on), still pictures and pushed frames every two seconds, so a low
`fps` there is normal. Sends over 400 ms are logged as `slow frame` in the log. Persistent slow
sends usually mean a poor cable or hub.

### Port 7781 in use

The menu shows *Not listening: port 7781 in use*. Another Marquee (often a `marquee serve` in a
terminal) is running; quit it and toggle **Allow Other Devices** or restart Marquee.

### Other devices get 401

Turn on **Allow Other Devices** and send a valid key. Check with `marquee key list` on the Mac
running Marquee that the key exists. `403` on `/v1/keys` or `/v1/settings` is expected from other
devices: those only work locally.

### Logs

The menu-bar app logs to `~/Library/Logs/Marquee/marquee.log`; every API call (except preview,
state and events) is logged with its status and time taken.

```bash
tail -f ~/Library/Logs/Marquee/marquee.log
```
