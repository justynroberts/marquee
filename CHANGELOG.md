# Changelog

## Unreleased

- Camera scenes from the webcam: `camera` (dot matrix), `popart` (posterised triptych), `motion`
  (trails where things move) and `slitscan` (time displacement). The camera opens only while one
  plays and is released 10 seconds after.
- Interactive camera scenes using Vision on the Mac: `snowfall` (snow settles on your
  silhouette), `bubbles` (touch to pop, with a counter) and `airpaint` (pinch to draw in the air).

## 0.1.0

First release.

- Drives the Thermalright Trofeo Vision 9.16 (0416:5408) over USB with its own driver, from a
  menu-bar app with a web remote, an HTTP API, a command line and an MCP server for Claude.
- Scenes, audio visualisers, the scrolling message board (colours, effects, fonts, sprites,
  emoji, live tokens), a full-resolution dashboard and interference field, GIF/WebP/image media,
  playlists (including your own), a daily schedule and a guided showcase.
- Now Playing for Music and Spotify, and karaoke with synced lyrics from LRCLIB; adjustable
  lyric timing.
- HUD mode: high-contrast clock, message or lyrics, mirrored to read in a screen's reflection.
- Alerts and timers, PagerDuty incidents, a news ticker, generic and GitHub webhooks.
- Output matches the original Python tools pixel for pixel (vendored FreeType and HarfBuzz).
- API keys for other devices; keyless access only for programs on this Mac, never web pages.
- Checks GitHub Releases for updates.
