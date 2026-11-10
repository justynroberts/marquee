# Changelog

## Unreleased

- Full-resolution `bigclock` and `worldclock` (cities from `world-clock.txt`), and `headlines`, a
  news wall of Hacker News and BBC News headlines in four slowly scrolling columns.
- `flights` (aircraft overhead on a radar, from adsb.lol), `quakes` (USGS earthquakes on a world
  map), `onthisday` (Wikipedia's history for today), `moon` (tonight's phase and the month ahead)
  and `countdown` (days until the dates in `countdown.txt`).
- `wordclock`, `calendar` (with UK bank holidays), `markets` (crypto prices), `air` (air quality
  and pollen) and the `flock` scene.
- `newsreel`: one news source crawling in mid-size type on two rows. Presets for BBC (news,
  world, technology, sport), Sky, The Guardian, Al Jazeera, NPR, Hacker News, The Verge and Ars
  Technica as `newsreel:NAME`; plain `newsreel` plays the source in `newsreel.txt` (any feed URL).
  News programs have their own group in the menu and the remote.
- `weather`: a full-resolution forecast (now, 24 hours, five days) for the place in `weather.txt`.
- Camera scenes `thermal` (heat camera), `kaleido` (kaleidoscope) and `ascii` (you in text).
- `snowfall` no longer fills the panel and freezes after a few minutes: settled snow melts.
- `snowfall` and `bubbles` show you brighter and in colour, levelled to the room's light.
- RSS feeds in `news-sources.txt` were read as empty; they work now.

## 0.2.0

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
