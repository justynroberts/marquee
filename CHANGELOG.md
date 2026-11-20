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
- `sun` (today's daylight where you are), `grid` (carbon intensity, generation mix and the
  greenest hours for your region), `aurora` (AuroraWatch UK and NOAA Kp), and the camera scenes
  `neon` and `clones`.
- `warroom`: a cold-war strategic simulation on a vector world map, every run ending with no
  winner.
- `claude`: Claude Code's token use on this Mac today and this week, and the sessions running
  now, read from its local history; and `claudehive`, `claudecity` and `claudeflow`, three
  animated takes on the same numbers, plus `claudeclock`, `claudeorbit`, `claudeburst` and
  `claudemeter`, and `claudelimits`, which shows how much of a Pro or Max plan is left (session and week, with resets and pace), and `claudediary`, Claude's diary written by a quill on a candle-lit page, the ink drying and then soaking away. `diary` is the same page as a news day book (weather, BBC and Hacker News headlines, on this day, the light, the moon, Claude, quotations), with three narrower books: `commonplace` (quotations), `naturediary` and `newsdiary`. Two handwriting fonts ship inside the app: Homemade Apple (Apache 2.0) and Meddon (SIL OFL), licences in `internal/dashboard/fonts/`. They have their own **Claude** section in the menu and the web remote.
- `widgets`: a board of readings pushed with `POST /v1/widgets` (MCP `set_widget`,
  `delete_widget`), plus a relay through an ntfy topic for tools that cannot reach the Mac.
- `claudelive`: each Claude Code session's current tool and the latest calls, from a hook
  (`scripts/claude-hook.sh`, `POST /v1/claude/activity`); the `claudehive` orbs show it too.
- The full-resolution screens use SF Mono Bold and brighter greys, which read better on the
  LEDs (the system `dashboard` keeps the regular weight); clocks and numbers use DIN, news and
  history Georgia, weather Rockwell and the playful Claude screens Avenir Next.
- Ten more screens: `progress` (time as progress bars), `pomodoro` (a focus timer, `focus.txt`),
  `netwatch` (latency, loss and DNS time), `status` (service status pages, `status.txt`), `rain`
  (the next two hours in 15-minute steps), `iss` (the space station's ground track), `launches`
  (upcoming rocket launches with a countdown), `orrery` (the planets now), `daynight` (the
  terminator on a world map) and `fx` (exchange rates, `fx.txt`). Status pages, currencies and
  the focus timer are in Settings.
- **Settings…** in the menu bar (and the web remote) opens a page for every info screen setting:
  location (UK postcode or place), world clock, newsreel, news feeds, coins, countdown, bank
  holidays, train station and key, grid region. Values are checked before they are saved and the
  page says what was understood. API: `GET /v1/config`, `PUT /v1/config/{id}`; MCP:
  `list_settings`, `set_setting`.
- `trains`: live departures and arrivals for a station from National Rail (`trains.txt`, Long
  Eaton by default), with a free Rail Data Marketplace key set in Settings.
- `weather.txt` takes a full UK postcode as well as a place name.
- Weather, flights, air quality and the board's `{weather}` use this Mac's location (Location
  Services, rounded to about a kilometre) when `weather.txt` and `@location` name no place.
- The full-resolution screens added in this release use a brighter palette with colour for
  meaning (altitude, magnitude, temperature, air quality, rises and falls).
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
