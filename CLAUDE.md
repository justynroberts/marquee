# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Marquee drives the Thermalright Trofeo Vision 9.16 USB LED panel (`0416:5408`, 1920 x 462) from a
Mac. One Go module, one binary (`marquee`) that is the menu-bar app (inside `Marquee.app`), the HTTP
server + web remote (port 7781), a CLI client, and an MCP server. `docs/DEVELOPMENT.md` is the
authoritative architecture/build/release doc — read it before non-trivial changes.

## Commands

```bash
brew install libusb go                          # cgo: needs Xcode CLT (FreeType, HarfBuzz, libusb, miniaudio)
go build -o bin/marquee ./cmd/marquee
go vet ./... && go test ./...                   # no panel, mic, network or music player needed
go test -run TestName -v ./internal/board       # single test
bin/marquee serve --no-panel                    # server + web remote without hardware
bin/marquee render NAME --seconds 3 --out f.png # render any program headlessly (dev tool, not in usage text)
scripts/release.sh --local                      # signed dist/Marquee.app to try the menu bar
```

`scripts/release.sh VERSION` tags, pushes and publishes a notarised GitHub release — only run it when
explicitly asked. `--dry-run VERSION` notarises without touching git/GitHub.

If linking fails with `Undefined symbols: _pipe2`, the active toolchain's SDK is older than the
Homebrew libusb (built for macOS 27). `xcode-select -p` should be `/Library/Developer/CommandLineTools`
(SDK 27.2), not an older Xcode.app.

libusb is linked statically from `/opt/homebrew/lib/libusb-1.0.a` (cgo flags in `internal/usb/usb.go`).
Release refuses binaries containing `/Users/` paths, so keep `-trimpath`.

## Architecture

- `cmd/marquee`: `main.go` dispatches subcommands; `app.go` owns engine, keys, listener, Bonjour,
  `settings.json`; `tray_darwin.go` is the menu bar (`fyne.io/systray`, dialogs via `osascript`);
  `mcp.go` is MCP over stdio JSON-RPC where **every tool is a call to the HTTP API** (it never touches
  the engine directly).
- `internal/engine`: single render loop goroutine. Frame source is the overlay (flash/timer) if any,
  else the program → HUD alert restyle → mirror → brightness → preview → rotate 180° → JPEG 4:4:4 →
  one-slot channel → USB goroutine. A `Program` has `Name()`, `Interval()` (min frame pacing; 0 = as
  fast as the panel, ~12 fps) and `Frame(t, dt) *image.RGBA`; returning `nil` means finished
  (overlay removed / program restarted). Panics in programs are caught and retried.
- Two drawing models: LED-grid programs (scenes, board, flash, timer, draw) draw into a `led.Grid`
  of 275 x 66 cells expanded to round 7 x 7 px dots; full-res programs (dashboard, interference, Now
  Playing, karaoke, HUD) draw the 1920 x 462 image directly.
- `internal/audio` and `internal/camera` open the mic/webcam lazily on first use and release them
  after 10 s idle; scenes read them through swappable `AudioSource` / `CameraSource` vars (tests
  fake these). A new hardware capability needs an entitlement in `scripts/entitlements.plist` and a
  usage string in the `Info.plist` written by `release.sh`, or macOS kills the app.
- `internal/scenes`: scenes self-register via `scenes.Register(name, group, description, factory)`.
- `internal/api`: routes in `api.go` (`auth` vs `localOnly` wrappers), key store, webhooks,
  Bonjour. `openapi.go` is a **hand-written** OpenAPI document — update it when routes change.
  Adding a user-facing feature typically means touching the API route, `openapi.go`, the MCP tool
  in `mcp.go`, possibly the CLI in `main.go` and `web/remote.html`, and `docs/API.md` / `docs/GUIDE.md`.
- Auth model: loopback requests from this Mac need no key; other devices need "Allow Other
  Devices" plus a `Bearer mq_…` key; browser pages are never keyless.
- USB wire protocol is in `docs/PROTOCOL.md`; board markup in `docs/BOARD.md`.

## Pillow parity (don't break it)

Output must match the original Python/Pillow implementation pixel for pixel. That's why FreeType
2.14.3 and HarfBuzz 14.5.0 are vendored under `internal/ft` (match Pillow 13's bundled versions —
don't upgrade casually), and why `internal/pil`, `internal/dashboard/pilline.go`, `engine.FitCover`
and media frame timing are ports of Pillow behaviour. Don't "simplify" these into stdlib
equivalents. Parity tests are opt-in via env vars pointing at Python reference output not in this
repo (`PARITY_DIR`, `TEXT_PARITY`, `EMOJI_PARITY`, `DASH_PARITY`, `MEDIA_PARITY`,
`MARQUEE_PARITY_DIR`, `MARQUEE_VIZ_PARITY` — table in `docs/DEVELOPMENT.md`); they skip otherwise.
`HUD_PNG=DIR`, `HIRES_PNG=DIR` and `NOWPLAYING_PNG=path` dump frames for eyeballing.

## Design

`DESIGN.md` records the look: panel uses near-black ground (12,13,11), paper ink (230,226,214) and
one vermilion accent (255,91,56) only for alerts/hot readings; the web remote is Terminal-editorial
(one mono typeface, phosphor-green accent, FintonLabs info button, reduced motion).
