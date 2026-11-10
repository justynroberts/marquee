# Marquee

Marquee drives the Thermalright Trofeo Vision 9.16 USB LED panel (`0416:5408`, 1920 x 462) from a
Mac. It plays animated scenes, audio visualisers, a scrolling message board with colour, emoji and
effects, a live system dashboard, Now Playing with karaoke lyrics, a mirrored heads-up display, a
news ticker, alerts, timers and pictures. It is one Go binary that is at once the menu-bar app, an
HTTP server with a web remote, a command-line client and an MCP server for Claude. The USB driver
is written from scratch ([docs/PROTOCOL.md](docs/PROTOCOL.md)); no vendor software is needed.

## Install

1. Download `Marquee-<version>.dmg` from
   [GitHub Releases](https://github.com/justynroberts/marquee/releases/latest).
2. Open the disk image and drag **Marquee** to **Applications**.
3. Open Marquee from Applications. A dot-matrix **M** appears in the menu bar; there is no Dock
   icon. Plug in the panel and it is picked up within a couple of seconds.

Requires macOS 12 or later on Apple silicon. The app is signed and notarised, so it opens without
a Gatekeeper warning.

macOS asks for permissions the first time a feature needs them:

| Permission | Asked when | Why |
|---|---|---|
| Microphone | an audio visualiser first plays | The visualisers analyse live sound. Nothing is recorded or sent anywhere. The microphone is released 10 seconds after the last visualiser stops. |
| Camera | a camera scene first plays | The camera scenes draw the webcam on the panel. Nothing is recorded or sent anywhere. The camera is released 10 seconds after the last camera scene stops. |
| Automation (Music, Spotify) | `nowplaying`, `karaoke` or the HUD first looks for a track | Marquee asks Music and Spotify what is playing. It never changes anything in them, and never launches a player that is not running. |
| Local Network | you turn on **Allow Other Devices** | Other devices can then reach the panel, and Marquee advertises itself over Bonjour. |

If you said no by mistake, see [Troubleshooting](docs/GUIDE.md#troubleshooting).

Marquee checks GitHub Releases once a day. When a newer version is out, the menu shows **Download Marquee x.y.z…**, which downloads the signed disk image; **Check for Updates…** checks now.

## Quick start

- **Menu bar → Play** lists everything that can play, grouped into Playlists, Panels, Scenes and
  Audio. Start with **Playlists → showcase**, a guided tour of every feature with a title card
  before each part.
- **Open Remote** opens the web remote at <http://127.0.0.1:7781>: play programs, flash an
  alert, add a board message, start a timer, draw, upload a picture and watch a live preview.
- The rest of the menu: Brightness, Pause, Upside Down (for a panel mounted the other way up),
  HUD Mode, HUD Lyrics, Clear Alert, Allow Other Devices, Require Key on This Mac, New API Key…,
  API Reference, Open at Login and Show Data Folder.

Your files live in `~/Library/Application Support/Marquee` (messages, schedule, playlists, media,
keys). Logs go to `~/Library/Logs/Marquee/marquee.log`.

## Features

- **Scenes**: matrix, terminal, fire, life, warp, plasma, rain, clock, bounce, fireworks,
  aquarium, drawn on a 275 x 66 grid of round LEDs.
- **Audio visualisers**: spectrum, mirror, waterfall, scope, pulse and meter, from the microphone.
- **Camera scenes**: camera, popart, motion and slitscan, from the webcam, and interactive ones
  that see you: snowfall (settles on you), bubbles (pop them) and airpaint (pinch to draw). Also
  thermal (a heat camera), kaleido (a kaleidoscope) and ascii (you, drawn in text).
- **Message board** (`board`): `messages.txt`, one message per line, with colours, backgrounds,
  effects, fonts, sizes, animated sprites, colour emoji, live tokens such as `{time}`, `{cpu}` and
  `{weather}`, and green or amber phosphor themes. See [docs/BOARD.md](docs/BOARD.md).
- **Dashboard** (`dashboard`): clock, CPU, memory, network and machine details at the panel's full
  resolution, so small text stays sharp.
- **Interference** (`interference`): drifting wave sources drawn as contour bands, full
  resolution.
- **Clocks** (`bigclock`, `worldclock`): a full-resolution clock as large as the panel allows, and
  clocks for several cities with day/night strips (`world-clock.txt` picks the cities).
- **Weather** (`weather`): now, the next 24 hours and the coming days from
  [Open-Meteo](https://open-meteo.com), for the place in `weather.txt`.
- **Headlines** (`headlines`): a news wall, every headline in small type in four slowly scrolling
  columns, from the same sources as the `news` ticker.
- **Flights** (`flights`): planes overhead on a radar, live from [adsb.lol](https://adsb.lol),
  with callsign, type, altitude, speed and distance.
- **Earthquakes** (`quakes`): the world's quakes from the USGS on a map, the latest listed.
- **On this day** (`onthisday`): Wikipedia's history for today, one event at a time.
- **Moon** (`moon`): tonight's phase drawn large, the next full and new moons, the coming month.
- **Word clock** (`wordclock`) and **calendar** (`calendar`, with UK bank holidays).
- **Markets** (`markets`): crypto prices from CoinGecko with the day's move and the week.
- **Air quality** (`air`): AQI, pollutants and pollen for the weather place, and the next 48 hours.
- **Flock** (`flock`): a murmuration of birds wheeling across the LED grid.
- **Countdown** (`countdown`): days to go until the dates in `countdown.txt`.
- **Newsreel** (`newsreel`): one source crawling across the panel in mid-size type on two rows.
  BBC News by default; `newsreel:sky`, `newsreel:hn`, `newsreel:guardian` and others pick a
  preset, and `newsreel.txt` takes any RSS or Atom URL.
- **Now Playing** (`nowplaying`): artwork, title, artist, album and a progress bar from Music or
  Spotify.
- **Karaoke** (`karaoke`): time-synced lyrics from [LRCLIB](https://lrclib.net), each line lighting
  up as it is sung. Only the title, artist, album and length are sent.
- **HUD mode** (`hud`): black with bold white type, a huge clock and one line, mirrored so it reads
  correctly reflected in your monitor. Can show lyrics instead of the clock.
- **News ticker** (`news`): Hacker News headlines, or any RSS and Atom feeds you list.
- **Alerts and timers**: flash text over whatever is playing, or run a countdown.
- **PagerDuty**: new triggered incidents flash on the panel; `{incidents}` and `{incident}` show
  the current state on the board.
- **Webhooks**: a generic hook that flashes any text, and a GitHub hook for failed workflow runs,
  releases, new issues and pull requests, merges and stars.
- **Playlists**: built-in `show`, `green` and `audio`, plus your own.
- **Schedule**: `schedule.txt` runs actions at set times every day.
- **Media**: upload PNG, JPEG, GIF or WebP (animated GIF and WebP play at their own timing), or
  push frames from your own app.
- **Drawing pad** (`draw`): paint on the LED grid from the web remote.
- **Showcase** (`showcase`): the tour.

The feature guide is [docs/GUIDE.md](docs/GUIDE.md).

## HTTP API

Everything the menu and remote can do is available over HTTP on port 7781. Requests from the Mac
running Marquee need no key. Requests from other devices need **Allow Other Devices** turned on and
an API key (menu → **New API Key…**, or `marquee key create NAME`), sent as
`Authorization: Bearer mq_…`.

```bash
M=http://127.0.0.1:7781
curl -X POST $M/v1/flash -H 'Content-Type: application/json' -d '{"text":"Build failed 🔥","seconds":10}'
curl -X POST $M/v1/play  -H 'Content-Type: application/json' -d '{"program":"matrix"}'
curl -X POST $M/v1/messages -H 'Content-Type: application/json' -d '{"text":"[green]Deployed :check: {time}"}'
curl -X POST $M/v1/frame -H 'Content-Type: image/png' --data-binary @chart.png
curl -N $M/v1/events
```

Full reference: [docs/API.md](docs/API.md). The running server describes itself at
`GET /v1/openapi.json`.

## Command line

The binary inside the app is `/Applications/Marquee.app/Contents/MacOS/marquee`. Link it onto your
`PATH` if you want to type `marquee`:

```bash
sudo mkdir -p /usr/local/bin
sudo ln -sf /Applications/Marquee.app/Contents/MacOS/marquee /usr/local/bin/marquee
```

```bash
marquee status                          # what is playing
marquee play NAME                       # show, green, audio, board, dashboard, matrix, ...
marquee flash TEXT [--seconds N] [--colour C]
marquee say TEXT                        # add a message to the board
marquee timer MINUTES [LABEL]
marquee hud on|off|clear|TEXT
marquee hud mirror horizontal|vertical|off
marquee hud lyrics on|off
marquee lyrics [earlier|later|SECONDS]  # lyric timing, 0.25 s steps
marquee send FILE                       # show an image now
marquee key create NAME | key list | key revoke ID
marquee mcp                             # MCP server for Claude
marquee serve [--addr ADDR] [--lan] [--no-panel] [--data DIR] [--require-key]
marquee info | colour RRGGBB | image FILE | bench   # direct to the panel, no server
marquee version
```

The client commands talk to the running server. `MARQUEE_URL` and `MARQUEE_KEY` point them at
another machine. The direct commands open the panel themselves, so they only work while Marquee is
not running.

## Use from Claude (MCP)

`marquee mcp` is a Model Context Protocol server on stdin/stdout. It talks to the running Marquee
over the HTTP API, so Claude can check the status, play programs, flash alerts, write the message
board, manage playlists, drive the HUD, show images and look at a live preview of the panel.

```bash
claude mcp add marquee -- /Applications/Marquee.app/Contents/MacOS/marquee mcp
```

Claude Desktop and remote panels: [docs/GUIDE.md](docs/GUIDE.md#mcp-claude-code-and-claude-desktop).

## Documentation

- [docs/GUIDE.md](docs/GUIDE.md): feature guide, data folder, network access, troubleshooting
- [docs/BOARD.md](docs/BOARD.md): message board markup
- [docs/API.md](docs/API.md): HTTP API reference
- [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md): architecture, building, testing, releasing
- [docs/PROTOCOL.md](docs/PROTOCOL.md): the panel's USB protocol

## Build from source

```bash
brew install libusb go
go build -o bin/marquee ./cmd/marquee      # libusb linked statically; FreeType and HarfBuzz are vendored
go test ./...
bin/marquee serve                          # server and web remote, without the menu bar
```

`scripts/release.sh --local` builds a signed `Marquee.app` in `dist/`. Details in
[docs/DEVELOPMENT.md](docs/DEVELOPMENT.md). `marquee serve` also runs on Linux with libusb; the
menu-bar app is macOS only.

## Credits and licence

MIT licence, see `LICENSE`. Made by FintonLabs. Not affiliated with Thermalright.

Output is pixel-compatible with the original Python version: text is rasterised with the same
FreeType and HarfBuzz versions Pillow uses, emoji come from Apple Color Emoji exactly as Pillow
draws them, and resampling and line drawing are ports of Pillow's own code.

Includes FreeType 2.14.3 (FreeType Licence), HarfBuzz 14.5.0 (Old MIT), algorithms ported from
Pillow 13 (HPND), and a JPEG encoder derived from Go's `image/jpeg` (BSD). Emoji: Apple Color Emoji
from the system on macOS; [Twemoji](https://github.com/jdecked/twemoji) (CC-BY 4.0) elsewhere.
Weather from [Open-Meteo](https://open-meteo.com); lyrics from [LRCLIB](https://lrclib.net);
aircraft from [adsb.lol](https://adsb.lol) (ODbL); earthquakes from the USGS; "on this day" from
Wikipedia (CC BY-SA); prices from CoinGecko; air quality from Open-Meteo (CAMS); bank holidays
from gov.uk (OGL). The
licence texts ship in `Marquee.app/Contents/Resources`.
