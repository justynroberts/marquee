# Development

Marquee is one Go module (`github.com/justynroberts/marquee`, Go 1.26) producing one binary,
`marquee`. Inside `Marquee.app` it runs as the menu-bar app; from a terminal it is the server, the
client and the MCP server.

## Layout

| Path | Responsibility |
|---|---|
| `cmd/marquee/main.go` | Command dispatch and usage text; `serve`; the HTTP client commands (`status`, `play`, `flash`, `say`, `timer`, `hud`, `lyrics`, `send`, `key`); the direct panel commands (`info`, `colour`, `image`, `bench`) |
| `cmd/marquee/app.go` | `App`: owns the engine, API keys, PagerDuty feed, HTTP listener and Bonjour advertisement; saves `settings.json` and restarts the listener when network settings change |
| `cmd/marquee/tray_darwin.go` | The menu-bar app (`fyne.io/systray`), dialogs through `osascript`, Open at Login as a LaunchAgent |
| `cmd/marquee/tray_other.go` | Stubs for other platforms |
| `cmd/marquee/mcp.go` | `marquee mcp`: MCP over newline-delimited JSON-RPC 2.0 on stdio; each tool is a call to the HTTP API |
| `cmd/marquee/render.go` | `marquee render NAME [--seconds N] [--out file.png] [--data DIR]`: run a program headlessly at 12 fps and write its last frame (a development tool, not in the usage text) |
| `internal/api` | HTTP routes, authentication, key store (`keys.json`), OpenAPI document, webhooks, Bonjour |
| `internal/engine` | The render loop and everything that plays: programs, overlays (flash, timer), playlists, showcase, news ticker, HUD mode, schedule, media decoding, brightness, mirroring, preview and events |
| `internal/scenes` | The scene registry and every LED-grid scene; `viz.go` holds the audio visualisers |
| `internal/board` | The message board: markup parser, linter, tokens, sprites, effects and phosphor themes |
| `internal/led` | The LED grid (275 x 66 cells of 7 x 7 px), palette, font resolution and text rasterisation, expansion to a 1920 x 462 image |
| `internal/ft` | cgo binding to vendored FreeType and HarfBuzz |
| `internal/pil` | Ports of Pillow's Lanczos resampling and premultiplication |
| `internal/emoji` | Colour emoji: Apple Color Emoji from the system on macOS, embedded Twemoji (`twemoji.zip`) otherwise |
| `internal/dashboard` | Full-resolution system dashboard, the interference visual, and a port of Pillow's line drawing (`pilline.go`) |
| `internal/nowplaying` | Now Playing and karaoke screens; Music and Spotify polling through `osascript`; LRCLIB lyrics |
| `internal/hud` | The HUD screen and the `Mirrored` transform |
| `internal/feeds` | Weather (Open-Meteo), PagerDuty, Hacker News and RSS/Atom; registers board tokens |
| `internal/audio` | Microphone capture (`malgo`, i.e. miniaudio), FFT, bands, beats and levels |
| `internal/panel` | Supported models, handshake, frame protocol, JPEG encoding, 180° rotation, pacing |
| `internal/usb` | Minimal cgo libusb binding: open by VID/PID, claim, bulk transfer, reset |
| `internal/jpeg444` | Go's `image/jpeg` encoder modified to emit 4:4:4 (no chroma subsampling) |
| `web` | The web remote, one embedded `remote.html` |
| `scripts/release.sh` | Build, sign, notarise, package and publish |
| `docs/PROTOCOL.md` | The panel's USB protocol |

## The pipeline

```
program.Frame(t, dt) ─► overlay? ─► HUD alert restyle ─► mirror ─► brightness ─► preview
   (*image.RGBA 1920x462)                                                         │
                                                                                  ▼
                                   USB goroutine ◄── chan (1) ◄── rotate 180° ─► JPEG 4:4:4
                                   SendJPEG + status                              (quality 70–88)
```

`Engine.Run` is one loop on its own goroutine:

1. Run the schedule if the minute has changed.
2. Pick the frame source: the overlay (flash or timer) if there is one, otherwise the program.
   A `Program` returns `nil` when it has finished: an overlay is then removed, a program restarts.
   A panicking program is caught, reported as `error`, and retried.
3. In HUD mode a flash is redrawn by `hud.Render` as a full-screen amber alert.
4. Apply the mirror (the global one, or the HUD's while the HUD plays) and software brightness.
5. Keep the frame for `/v1/preview.jpg` (encoded only when someone asks for it).
6. Unless paused, `send`: open the panel if needed (retrying every two seconds), rotate 180°
   when the model's scan order and the user's flip disagree, encode a baseline 4:4:4 JPEG, and
   hand it to the USB goroutine through a one-slot channel, so encoding the next frame overlaps
   sending this one.
7. Pace: each program declares a minimum `Interval` (0 for as fast as the panel accepts frames,
   which is about 12 fps; 500 ms for the dashboard; 2 s for stills). Without a panel the loop runs
   at about 12 fps.

The USB goroutine sends each JPEG and waits for the panel's status reply; a failure closes the
panel and the next frame reconnects. JPEG quality starts at 88, drops by 4 (to no lower than 70)
when a frame exceeds 1 MB, and creeps back up when frames are under 768 KB. Encode and send times
are averaged every two seconds into `Status.Timing`, visible in `/v1/state`.

LED-grid programs (scenes, board, flash, timer, draw) draw into a `led.Grid` of 275 x 66 cells,
which `Grid.Image` expands into round dots on a ground colour. Full-resolution programs
(dashboard, interference, Now Playing, karaoke, HUD) draw the 1920 x 462 image directly.

The wire protocol (handshake, the JPEG split into 512-byte packets, the status reply) is in
[PROTOCOL.md](PROTOCOL.md).

Events (`state`, `overlay`, `panel`, `error`) are published to subscribers through buffered
channels; a slow subscriber drops events rather than blocking the loop. The SSE endpoint and the
menu bar are both subscribers.

## Pillow parity

Marquee replaced a Python implementation (Pillow, `board.py`, `viz.py`, `panel.py`, `stream.py`),
and its output is meant to be identical pixel for pixel:

- **Text** goes through `internal/ft`: FreeType 2.14.3 and HarfBuzz 14.5.0, the versions Pillow 13
  bundles, vendored as source under `internal/ft/freetype` and `internal/ft/harfbuzz` and compiled
  by cgo (`ftmod_*.c` and `harfbuzz.cc` are the unity-build entry points). Shaping follows Raqm as
  Pillow uses it, and layout follows `ImageDraw.text`: hinted glyphs, greyscale anti-aliasing,
  integer pen positions, the `la` anchor. The `pixel` font reproduces Pillow's built-in bitmap
  font. Without the system fonts, `led` falls back to the embedded Go fonts so a bare binary still
  renders text.
- **Emoji** are read from the 160 ppem bitmap strike of the Mac's own Apple Color Emoji, drawn on
  a transparent canvas, cropped to their ink and Lanczos-resized, as the Python board did.
- **Resampling** (`internal/pil`) and **line drawing** (`internal/dashboard/pilline.go`) are ports
  of Pillow's C code. `engine.FitCover` scales and crops exactly like the Python `fit_cover`.
- **Media** frame timing follows Pillow's rules (missing or zero delay = 100 ms, floor 50 ms, cap
  600 frames).

### Parity tests

The ordinary tests run without any of this. The parity tests are opt-in: each is skipped unless
its environment variable points at output from the Python reference harness (not in this
repository).

| Variable | Test | Expects in the directory |
|---|---|---|
| `PARITY_DIR` | `internal/board` `TestParityRender` | `cases/*.txt`; writes frames to `go/` |
| `TEXT_PARITY` | `internal/led` `TestTextParityWithPillow` | `cases.json` and reference PNGs |
| `EMOJI_PARITY` | `internal/emoji` `TestAppleParity` | `cases.json`, `*-lit.png`, `*-rgb.png` |
| `DASH_PARITY` | `internal/dashboard` `TestParityWithPython` | `py.png`; writes `go.png` |
| `MEDIA_PARITY` | `internal/engine` `TestMediaParityWithPillow` | `t.gif`, `t.webp`, `t.png`, `py.json`, `py-*.png` |
| `MARQUEE_PARITY_DIR` | `internal/scenes` `TestParityRender`, `TestParityMatrixAtlas`, `TestParityText`, `TestTerminalEchoParity` | writes frames, glyph atlases and text masks; reads `terminal_cases.json` |
| `MARQUEE_PARITY_SCENES` | with the above | comma-separated scene names to limit the run |
| `MARQUEE_PARITY_RUNS` | with the above | number of runs |
| `MARQUEE_VIZ_PARITY` | `internal/scenes` `TestVizParityHarness` | `signal.f32`; writes `go_*.png` and `go_log.json` |

```bash
TEXT_PARITY=~/parity/text go test -run TestTextParityWithPillow -v ./internal/led
```

Other opt-in variables for looking at output by eye or hitting real services:

| Variable | |
|---|---|
| `HUD_PNG=DIR` | `internal/hud` tests write each HUD case as a PNG |
| `NOWPLAYING_PNG=/path/out.png` | `internal/nowplaying` writes sample Now Playing and lyrics frames |
| `NOWPLAYING_REAL=1` | `internal/nowplaying` tests talk to the real Music/Spotify and LRCLIB |

`marquee render NAME --seconds 3 --out frame.png` renders any program offline for comparison.

## Building

Requirements: macOS on Apple silicon, Go 1.26, Xcode command line tools (cgo needs a C and C++
compiler for FreeType, HarfBuzz, libusb and miniaudio), and Homebrew libusb.

```bash
brew install libusb go
go build -o bin/marquee ./cmd/marquee
bin/marquee serve            # or: bin/marquee serve --no-panel, without hardware
```

libusb is linked statically from `/opt/homebrew/lib/libusb-1.0.a` (see the cgo flags in
`internal/usb/usb.go`), so the binary has no runtime dependency on Homebrew. On Linux it links
libusb through `pkg-config` and the menu-bar app is not built; `serve` works.

To try the menu-bar app without packaging, use `scripts/release.sh --local` and open
`dist/Marquee.app`; outside an app bundle the binary defaults to `serve`.

## Testing

```bash
go vet ./...
go test ./...
```

The tests need no panel, microphone, network or music player: USB is not touched, the audio engine
is fed synthetic samples, HTTP services are faked, and token values are frozen where frames are
compared. API tests exercise the handler directly, including loopback and network authentication.

## Releasing

`scripts/release.sh` does the whole release on this Mac; CI has neither the signing certificate
nor the notary profile. It needs a **Developer ID Application** certificate in the keychain and a
`notarytool` keychain profile named `notarytool`:

```bash
xcrun notarytool store-credentials notarytool --apple-id YOU@example.com --team-id TEAMID
```

| Command | Does |
|---|---|
| `scripts/release.sh --local [VERSION]` | Tests, build, sign (Developer ID if present, ad hoc otherwise), smoke test. Leaves `dist/Marquee.app`. Version defaults to `0.0.0-dev` |
| `scripts/release.sh --dry-run VERSION` | The above, then notarise and staple the app, build, sign, notarise and staple `dist/Marquee-VERSION.dmg`, and verify it as a downloader would. No git, no GitHub |
| `scripts/release.sh VERSION` | The above, then tag `vVERSION`, push the branch and tag, and create the GitHub release with the DMG |

Steps in order:

1. **Refuse first** (not with `--local`): no Developer ID certificate, a notary profile that does
   not answer, and for a full release a dirty working tree or an existing `vVERSION` tag all stop
   the script before anything is built.
2. `go vet ./...` and `go test ./...`.
3. Build for arm64 with `-trimpath` and `-X main.version=VERSION`; write `Info.plist` (bundle ID
   `com.fintonlabs.marquee`, `LSUIElement`, macOS 12 minimum, the microphone, local network and
   Apple Events usage strings, `_marquee._tcp` Bonjour service); copy the icon and licence files
   into `Resources`.
4. Sign with the hardened runtime and `scripts/entitlements.plist` (audio input, Apple Events),
   then `codesign --verify --strict`.
5. Refuse a binary that contains `/Users/` paths.
6. Smoke test: copy the app to a temporary folder, check `marquee version`, run
   `serve --no-panel` on port 7799, and check `/v1` answers and a preview frame renders.
7. Notarise and staple the app, then build the DMG (with an Applications link), sign, notarise and
   staple it too.
8. Verify: copy the DMG, mark it quarantined, and check `stapler validate` and `spctl` accept both
   the image and the mounted app as *Notarized Developer ID*.
9. Publish (full release only): tag, push, `gh release create`.

If notarisation is refused the script prints Apple's log for the submission.

## Licences

Marquee is MIT (`LICENSE`). Vendored and ported code keeps its own licence; the release copies each
licence file into `Marquee.app/Contents/Resources`.

| Component | Where | Licence | File |
|---|---|---|---|
| FreeType 2.14.3 | `internal/ft/freetype` | FreeType Licence (FTL) | `internal/ft/freetype/FTL.TXT` |
| HarfBuzz 14.5.0 | `internal/ft/harfbuzz` | Old MIT | `internal/ft/harfbuzz/COPYING` |
| Pillow 13 ports | `internal/pil`, `internal/dashboard/pilline.go`, parts of `internal/led` | HPND | `internal/pil/LICENSE-PILLOW` |
| Go `image/jpeg` derivative | `internal/jpeg444` | BSD 3-clause | `internal/jpeg444/LICENSE-GO` |
| Twemoji graphics | `internal/emoji/twemoji.zip` | CC-BY 4.0 | `internal/emoji/LICENSE-TWEMOJI` |

Go module dependencies: `gen2brain/malgo` (public domain), `grandcat/zeroconf` (MIT),
`shirou/gopsutil` (BSD), `fyne.io/systray` (Apache 2.0), `golang.org/x/image` (BSD).

Apple Color Emoji is read from the user's own system at run time and is not distributed.
The emoji attribution string (`emoji.Attribution`) appears in the About box.
