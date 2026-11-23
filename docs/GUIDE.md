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
| `neon` | (camera) You traced in glowing outlines whose colour drifts across the panel, with light trails |
| `clones` | (camera) Delayed copies of you, each in its own colour, trailing every move |
| `flock` | A murmuration: birds wheeling and turning together, scattering when a hawk crosses |

**Full-resolution screens** are drawn at the panel's native 1920 x 462, so small text stays sharp:

| Name | |
|---|---|
| `dashboard` | Clock, CPU (big percentage, two-minute trace, per-core bars), memory, network and machine details |
| `interference` | Three drifting wave sources drawn as contour bands, with a clock and fps label |
| `bigclock` | Hours and minutes as large as the panel allows, seconds beside them, the date, week and zone on the left, and a seconds sweep along the bottom |
| `flipclock` | A split-flap board: big tiles for the hours and minutes, small ones for the seconds, and each flap falls over its hinge, darkening as it turns, when its digit changes |
| `nixieclock` | Six nixie tubes: the lit numeral glows orange in front of the dark stack of the other nine, behind the mesh, with neon dots between the pairs blinking the seconds |
| `dialclock` | The day as a radio tuning dial: a backlit scale of hours slides past a fixed red needle, with the times of day banded along the top and a fine scale of seconds beneath |
| `swarmclock` | The time drawn in four thousand particles that hang breathing in the shape of the digits, then burst and stream to the new time when the minute turns; sixty dots count the seconds |
| `worldclock` | One column per city: time (dimmed at night), date, offset from here, and a 24-hour strip with daylight lit |
| `weather` | Temperature and conditions now, a 24-hour temperature curve over rain-chance bars, and five days ahead. Readings of 30°C and up are in vermilion |
| `flights` | Aircraft overhead: a radar that zooms in on busy skies, planes moved on between fetches by their own speed and track, and the nearest airborne ones listed with type, altitude, speed, distance and bearing. An emergency squawk is in vermilion |
| `quakes` | Earthquakes: the last day's M2.5+ and last 30 days' M4.5+ on a world map, which between them trace the plate boundaries, and the latest listed. M6+ is in vermilion; a quake in the last hour pulses |
| `onthisday` | Wikipedia's "on this day": one event every 12 seconds, its year large on the left |
| `warroom` | A cold-war strategic simulation on a vector world map: DEFCON climbs, two blocs trade growing waves of missiles, impacts bloom and the counters run, and every run ends with no winner before the next scenario begins (about 45 seconds each) |
| `sun` | Today's daylight where you are: the sun's path over the horizon with where it is now, dawn, sunrise, noon, sunset and dusk, and how much longer or shorter the day is than yesterday |
| `claude` | Claude Code on this Mac: today's output, input and cache tokens and API calls, output by the hour, the last seven days, the sessions running now (working ones pulse) with each conversation's size, and today's projects |
| `claudelive` | What Claude Code is doing this moment, from its hooks: each session's current tool large with the file, command or search it is on, and the latest calls (see "Live Claude activity") |
| `widgets` | A board of readings posted by your scripts, CI or cloud tools (see "Widgets") |
| `claudehive` | Each running Claude Code session as an orb ringed by how full its conversation is: working ones spin the Claude spark and throw sparks across to the big spark (which spins faster the more tokens flow), idle ones doze. A rolling counter of today's output |
| `claudecity` | Your week with Claude Code as a city at night: a district a day, a tower an hour as tall as its output, windows lit; this hour's tower goes up under a crane with a blinking light |
| `claudeflow` | Today's output as a river, one band per project as wide as its output each hour, with sparks drifting downstream as fast as tokens are arriving |
| `claudeclock` | The Claude spark as a 24-hour clock filling the panel: one ray an hour, as long as that hour's output, midnight at the top; the hour now breathes as tokens arrive and a hand sweeps round. Today's total large on the left, the peak hour and busiest project on the right |
| `claudeorbit` | The spark as a sun with each running session a planet in orbit: size is today's output, the ring how full the conversation is; working planets race round and stream sparks into the sun |
| `claudeburst` | A firework for every reply Claude writes, as big as its output and in its project's colour, labelled where it bursts; when nothing new arrives it replays the day's replies, dimmer |
| `claudemeter` | Three big gauges: output tokens a minute, the fullest running conversation's context, and the cache hit rate; needles swing to each reading and tremble while Claude works |
| `claudelimits` | How much of your Claude Pro or Max plan is left: the five-hour session and the week as big "percent left" figures over fuel bars, with reset times, a pace mark (how far through the window you are, so you can see if you are burning ahead of it) and the week split by app. It reads the login Claude Code keeps in your Keychain and asks Anthropic for the same figures `/usage` shows, every 3 minutes; the token goes only to api.anthropic.com and is never stored or shown. If Claude Code's login has lapsed, open Claude Code once to renew it |
| `claudediary` | Claude's diary on an old candle-lit page. A quill writes each entry in real handwriting: the nib follows the strokes, slows over letters, lifts across the gaps and draws the underline. The iron-gall ink goes on blue-black and dries to sepia, the odd blot drops from the nib, and after a while the ink soaks back into the paper until the page is blank for the next entry. Entries rotate: the plan left (and whether you are pacing yourself), today's work (replies, projects, when you began and your busiest hour, which models), the latest calls in words ("At 14:02 I edited claudediary.go in marquee"), what each running session is doing, a tally of today's edits, commands, reads and searches (from the hooks), the week, and now and then a quotation copied out and signed in red |
| `diary` | The same page as a day book: the weather where you are, two BBC headlines, the Claude plan, on this day in history (the year written large), two Hacker News stories, the light (sunrise, sunset, how much shorter or longer the day is than yesterday, and how far off the solstice is), the moon, today's Claude work, and two quotations from writers long out of copyright. Headlines and quotations move on each time round |
| `commonplace` | A commonplace book: one quotation after another, copied out by the quill and signed in red |
| `naturediary` | A nature diary: the weather, the light (sunrise, sunset, the days drawing in or out), the moon and the odd quotation |
| `newsdiary` | The day's papers written up by hand: BBC and Hacker News headlines and on this day in history |
| `progress` | Time as progress bars ticking live: this hour, today, this week, month, quarter and year (the year's percentage large), with day of the year, ISO week, days left and working days left this month |
| `pomodoro` | A focus timer: a 25-minute session starts when it plays, then a 5-minute break, with a long break every fourth. Huge minutes and seconds, the phase in colour, the cycle's sessions and what comes next. Lengths in `focus.txt` |
| `netwatch` | Internet health, measured every two seconds: connect time and loss to your router, Cloudflare (1.1.1.1) and Google (8.8.8.8), and DNS lookup time, with five minutes of history and ONLINE, SLOW or DOWN large |
| `daynight` | The world in day and night: the terminator and twilight bands on a map, where the sun and moon are overhead, your place, and the world-clock cities with their local times |
| `orrery` | The solar system now from above (distances compressed so all eight planets fit), each planet's distance from Earth in AU and light-minutes, and which are in the evening or morning sky |
| `moon` | Tonight's moon drawn to its phase, illumination and age, the next full and new moons, and the next 30 nights |
| `wordclock` | The time to the nearest five minutes spelled out in a grid of letters, with dots for the minutes in between |
| `calendar` | This month with ISO week numbers, today in ink and bank holidays boxed; next month small and the next three bank holidays |
| `markets` | Up to four cryptocurrencies from CoinGecko: price, the day's move (vermilion at 5% or more) and the last seven days |
| `grid` | The electricity grid where you are (Great Britain): grams of CO2 per kWh now, the generation mix, the next 48 hours and the greenest three hours to run something heavy |
| `aurora` | Will you see the northern lights? AuroraWatch UK's alert level, the last 24 hours of disturbance, and NOAA's Kp index past and forecast; a green curtain shimmers along the top at amber or red |
| `iss` | The International Space Station on a world map: where it is, its track for the last and next 45 minutes, height, speed, whether it is in sunlight, and how far it is from you |
| `launches` | Rocket launches coming up worldwide: a live T-minus countdown to the next with rocket, provider, pad and status, and the ones after it |
| `rain` | Will it rain? A verdict large (dry for two hours, rain from 14:30, rain until 15:15), rain in 15-minute steps for the next two hours, and the next 12 hours |
| `status` | Are the services you rely on up? Each status page's state in green, amber or vermilion, and the incidents for anything degraded. List in `status.txt` |
| `fx` | Exchange rates: up to six currencies priced in your base (GBP by default) with the day's move and the last 30 days. Currencies in `fx.txt` |
| `trains` | A National Rail station board: the next departures and arrivals with platform and running (green on time, amber late, vermilion cancelled) and the reason for a delay. Needs a free Rail Data Marketplace key |
| `air` | Air quality: the European AQI, PM2.5, PM10, NO2 and ozone against WHO guideline levels, the next 48 hours, and pollen in Europe. Poor air is in vermilion |
| `countdown` | Days until the next date in `countdown.txt` (hours, minutes and seconds on the day), and the ones after it |
| `newsreel` | One news source crawling across the panel in mid-size type, two rows, alternate stories on each; `newsreel:bbc`, `newsreel:sky` and the other presets below pick a source |
| `headlines` | A news wall: Hacker News and BBC News headlines in small type, four columns scrolling slowly upwards; Hacker News stories with 500+ points are marked in vermilion |
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
| `snowfall` | Snow falls and settles on your head and shoulders; move and it slides off or is shaken loose |
| `bubbles` | Bubbles rise across the whole panel; touch one with any part of you to pop it. Counts your pops |
| `airpaint` | Pinch thumb and index finger to draw light in the air; open them to lift the pen. Strokes fade over a few seconds; two hands draw in two colours |
| `thermal` | A heat camera in an iron palette: you glow white-hot against a cold room |
| `kaleido` | The webcam through an eight-way kaleidoscope that turns and breathes slowly |
| `ascii` | You, drawn in text at full resolution: each character's density and colour come from the picture |

The interactive scenes and `thermal` use Apple's Vision framework on the Mac itself: person segmentation for
`snowfall`, `bubbles` and `thermal`, hand pose for `airpaint`. It runs only while one of them plays, and
nothing leaves the Mac. Stand back far enough that your head and shoulders are in frame; plain
backgrounds and good light help.

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

## Settings

Menu → **Settings…** (or **Settings** at the top of the web remote) opens a page on this Mac for
everything the info screens show: your location (a UK postcode or a place), the world clock
cities, the newsreel source, the news feeds, the coins, the countdown dates, the bank holidays,
the train station and train times key, and the grid region. Each setting is checked when you save
it: a postcode or place is looked up, a feed is fetched, a time zone is read. If it is wrong the
page says why and nothing changes; if it is right, the line underneath says what Marquee
understood, such as `Westminster  51.50, -0.14  ·  grid region SW1A`. **Use default** goes back to
the default. The buttons beside each setting put the screens that use it on the panel.

The page only works from this Mac, like API keys. It writes the same text files described below,
so editing them by hand still works.

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

`headlines` uses the same fetch but shows more: the top 40 Hacker News stories and up to 25 items
per feed, against the ticker's 15 and 10, mixed so each source takes turns. Without a
`news-sources.txt` it also shows the BBC News front page; with one, it shows exactly the sources
listed.

`newsreel` shows one source on its own. `newsreel:NAME` plays a preset; plain `newsreel` plays the
first line of `newsreel.txt` in the data folder, which may be a preset name or any RSS or Atom URL
(BBC News when there is no file). It fetches 30 stories and refreshes every ten minutes, taking
new stories in as each row comes round to its start.

| Preset | Source |
|---|---|
| `bbc` | BBC News front page |
| `bbc-world`, `bbc-tech`, `bbc-sport` | BBC World, Technology, Sport |
| `sky` | Sky News |
| `guardian` | The Guardian (UK) |
| `aljazeera` | Al Jazeera |
| `npr` | NPR News |
| `hn` | Hacker News (500+ point stories get a vermilion marker) |
| `verge`, `ars` | The Verge, Ars Technica |

```
# newsreel.txt
https://www.theregister.com/headlines.atom
```

`worldclock` shows San Francisco, New York, London, Tokyo and Sydney. To choose your own (up to
eight), create `world-clock.txt` in the data folder with one city per line, as `Label = zone` or a
bare [time zone name](https://en.wikipedia.org/wiki/List_of_tz_database_time_zones):

```
# world-clock.txt
Home = Europe/London
Asia/Singapore
Austin = America/Chicago
```

`weather` uses [Open-Meteo](https://open-meteo.com) (no key). It shows the place named on the
first line of `weather.txt` in the data folder (a place name, or a full UK postcode, which is
looked up with [postcodes.io](https://postcodes.io) and also sets the `grid` region), else the message board's `@location`, else where
this Mac is, else London. It refreshes every 15 minutes.

Where this Mac is comes from Location Services: macOS asks once, the first time a screen needs it.
Marquee asks for an approximate fix (about a kilometre), has macOS name the town, and keeps the
last fix in `location.json` so it is there at once after a restart; it asks again every three
hours. The coordinates are rounded to two decimals (about a kilometre) before they are sent to
Open-Meteo or adsb.lol. The board's `{weather}` and `{temp}` use it too when messages.txt has no
`@location`. Turn it off in System Settings > Privacy & Security > Location Services, or name a
place in `weather.txt`.

`flights` shows the aircraft within 40 nautical miles of the same place, from
[adsb.lol](https://adsb.lol) (no key), every 15 seconds. The place's coordinates are sent to
adsb.lol, as they are to Open-Meteo for the weather. `quakes` reads the
[USGS](https://earthquake.usgs.gov/earthquakes/feed/) feeds every five minutes and `onthisday`
reads [Wikipedia](https://api.wikimedia.org) once a day; neither sends anything about you. `moon`
is worked out on the Mac from the average lunar month, so its full and new moon times can be out
by up to half a day.

`air` uses the weather place too, and Open-Meteo's air quality forecast (CAMS); pollen is only
forecast in Europe.

`markets` shows Bitcoin, Ethereum and Solana in US dollars. To choose, create `markets.txt` with up
to four [CoinGecko](https://www.coingecko.com) coin ids (the name in the coin page's address) and
optionally a currency. Prices refresh every two minutes.

```
# markets.txt
bitcoin
dogecoin
currency = gbp
```

`trains` shows Long Eaton (LGE) until you put another station's three-letter code on the first
line of `trains.txt` (Nottingham is NOT, Derby DBY). The times are National Rail's own (Darwin),
which are free but need a key:

1. Register at [raildata.org.uk](https://raildata.org.uk) (free).
2. Find the **Live Arrival and Departure Boards** product and subscribe (free).
3. Copy the **Consumer key** from the product's Specification tab, open menu → **Settings…** and
   paste it into **Train times key**. A new key can take half an hour to start working.

The key is saved as `rail-key` in the data folder, readable only by you, and is sent only to the
Rail Data Marketplace. The board refreshes every minute and covers the next two hours.

`grid` uses the [Carbon Intensity API](https://carbonintensity.org.uk) (no key) for the region of
this Mac's postcode district: the district comes from Location Services, or from
[postcodes.io](https://postcodes.io) given the rounded coordinates. Put a district such as `SW1A`,
or `gb` for the whole country, in `grid.txt` to choose. `aurora` reads
[AuroraWatch UK](https://aurorawatch.lancs.ac.uk) and NOAA's
[Kp forecast](https://www.swpc.noaa.gov) every ten minutes. `sun` is worked out on the Mac.

`status` watches GitHub, Cloudflare, Anthropic, OpenAI, Atlassian and Vercel until you list
services in `status.txt`, one per line: a preset name, or `Name = https://status.example.com`
for any site run on Atlassian Statuspage. It checks every two minutes.

`fx` prices USD, EUR, JPY, CHF, AUD and CAD in pounds; put a line such as `EUR: USD GBP` in
`fx.txt` to change that. Rates are the European Central Bank's, published once each working
day, via [Frankfurter](https://frankfurter.dev).

`pomodoro` runs 25 minutes of focus and 5 of break, with a 15-minute break after every fourth
session. `focus.txt` takes the minutes: `50 10`, or `50 10 30 3` for the long break and how
often it comes.

`claude` reads the conversation history Claude Code keeps in `~/.claude/projects` and its
running sessions in `~/.claude/sessions`: the first time it reads the last week, then only what
was added, every 20 seconds. Nothing is sent anywhere. Token counts are what the API reported
for each reply; they are not your plan's usage limits, which Claude Code does not store.

`iss` reads [wheretheiss.at](https://wheretheiss.at) every few seconds and `launches` reads
[Launch Library 2](https://thespacedevs.com) every half hour. `rain` uses Open-Meteo for the
weather place. `progress`, `daynight`, `orrery` and `netwatch` are worked out on the Mac
(`netwatch` sends only TCP connection attempts, a ping to your router and DNS lookups).

`calendar` marks bank holidays in England and Wales from gov.uk. Put `scotland`,
`northern-ireland` or `none` in `calendar.txt` to change that.

`countdown` counts down to Christmas and New Year until you create `countdown.txt` in the data
folder, one date per line. A date without a year repeats every year; a time is optional:

```
# countdown.txt
Holiday = 2027-07-14
Launch = 2027-02-01 09:00
Birthday = 03-14
```

A one-off date drops off the day after it passes. It is read when the countdown starts, so play it
again after editing.

The city whose offset matches this Mac's is labelled HERE. It is read when the clock starts, so
play it again after editing.

Fetch now with `POST /v1/news/refresh` (or the MCP `refresh_news` tool). The ticker is the message
board reading `news.txt`, which Marquee rewrites; editing `news.txt` yourself is pointless. If every
source fails, the ticker says the headlines are unavailable.

## Widgets: push anything to the panel

The `widgets` screen shows readings that your own scripts, CI jobs or cloud tools post: a
label, a value in green, amber or vermilion, and a line of text. Post the same id again to
update it.

```bash
curl -d '{"id":"build","label":"CI","value":"PASS","status":"ok"}' localhost:7781/v1/widgets
curl -d '{"id":"queue","label":"Queue","value":"42","unit":"jobs"}' localhost:7781/v1/widgets
curl -X DELETE localhost:7781/v1/widgets/queue
```

`status` is `ok`, `warn`, `bad` (pulses) or `info`; `ttl` (seconds) drops a reading when it goes
stale; `flash: true` also flashes it. Other devices need an API key, as for any route. From
Claude, the MCP tools `set_widget` and `delete_widget` do the same.

**From the cloud (Grok Bot and the like).** A tool running elsewhere cannot reach this Mac, so it
posts to a relay instead:

1. Make up a long random topic name, such as `marquee-` followed by 12 random letters. Anyone who
   knows it can post, so treat it like a password.
2. Open menu → **Settings…** and paste `ntfy.sh/<topic>` into **Widget relay**.
3. Add a final step to the cloud tool's routine that runs
   `curl -d '{"id":"grok","label":"Grok Bot","value":"DONE","status":"ok","text":"…","flash":true}' ntfy.sh/<topic>`.

Marquee checks the topic every 20 seconds. Plain text works too: `curl -H "Title: Grok Bot" -d
"Retro drafted" ntfy.sh/<topic>` becomes a widget called Grok Bot. Messages pass through
ntfy.sh, so don't send anything confidential.

## Live Claude activity

With Claude Code's hooks pointed at Marquee, `claudelive` shows what each session is doing this
moment (its tool large, with the file, command or search it is working on, and a feed of the
latest calls), and the `claudehive` orbs say what each session is running. Install the hook:

1. Copy `scripts/claude-hook.sh` from this repository to `~/.claude/hooks/marquee-activity.sh`
   and make it executable. It needs `jq`.
2. Add it to `~/.claude/settings.json` for four events, each as
   `{"type": "command", "command": "$HOME/.claude/hooks/marquee-activity.sh MODE", "async": true}`:
   `PreToolUse` with `tool`, `PostToolUse` with `done`, `UserPromptSubmit` with `prompt` and
   `Stop` with `stop`.

The hook sends only the session id, the project folder, the tool's name and a short detail (a
file's name, a command's description, a search pattern) to Marquee on this Mac. Your messages to
Claude are not sent.

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

Rest the pointer on Marquee's menu-bar icon and a live mini view of the panel opens at the top centre of the screen,
with what is playing and whether a panel is connected. Everything renders without a panel, so
this is also how to use Marquee with no panel plugged in (the web remote shows the same view).

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
Folder**). The info screens' files can all be set from **Settings…** instead. On Linux it is `~/.config/marquee`; `marquee serve --data DIR` uses another folder.

| File | |
|---|---|
| `messages.txt` | The message board. Yours to edit |
| `schedule.txt` | The daily schedule. Yours to edit |
| `news-sources.txt` | News ticker and headlines sources. Yours to create |
| `grid.txt` | Postcode district (or `gb`) for `grid`. Yours to create |
| `trains.txt` | Station code for `trains`. Yours to create |
| `rail-key` | Rail Data Marketplace key for `trains` (mode 0600), saved from Settings |
| `markets.txt` | Coins and currency for `markets`. Yours to create |
| `calendar.txt` | Bank holiday area for `calendar`. Yours to create |
| `countdown.txt` | Countdown dates. Yours to create |
| `location.json` | This Mac's last location fix, written by Marquee |
| `newsreel.txt` | The newsreel's source: a preset name or feed URL. Yours to create |
| `world-clock.txt` | World clock cities. Yours to create |
| `weather.txt` | Where the weather, air, flights, sun and grid screens are for: a place such as `Manchester` or a UK postcode. Yours to create |
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
| `list_settings` | The info screens' settings and their values |
| `set_setting` | Check and save one setting (`id`, `value`) |
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
