# Message board markup

The `board` program scrolls `messages.txt` from the data folder
(`~/Library/Application Support/Marquee/messages.txt`) across the panel as a dot-matrix sign:
a grid of 275 x 66 LEDs. Messages follow one another in a single line, separated by a gap.

Edit the file in any text editor (menu → **Show Data Folder**), in the web remote, with
`PUT /v1/messages`, or one line at a time with `marquee say TEXT` or `POST /v1/messages`. The
board notices a change within a second; there is nothing to reload. Time-based tokens refresh
every minute.

```
# A comment
@step=3
[green]Build passed[/] :check:   [big]{time}[/]
[bg=blue][white] ON AIR [/][/]
! Production is down
```

## Lines

| Line starts with | Meaning |
|---|---|
| `#` | Comment, ignored |
| `@` | A board-wide setting, `@key=value` (see [Settings](#settings)) |
| `!` | An alert: vermilion and pulsing, unless tags in the line say otherwise |
| anything else | A message |

Blank lines are ignored. If the file has no messages the board shows `(messages.txt is empty)`.
If the file does not exist it is created with a set of examples.

## Tags

A tag is a word in square brackets. It changes the style from that point on. `[/]` closes the
most recent tag and goes back to the style before it; `[]` and `[reset]` do the same. Tags nest:

```
[wave][green]Build passed[/][/] and back to normal
```

Tags cannot contain spaces. Anything in brackets that is not a valid tag (`[todo]`,
`[font=Helvetica Neue]`) is shown as ordinary text, and the API returns a warning for it. Tag names are not case-sensitive.

### Colours

| Tag | Colour |
|---|---|
| `[white]` | warm white (default text) |
| `[grey]` | grey |
| `[red]` | red |
| `[orange]` | orange |
| `[yellow]` | yellow |
| `[green]` | green |
| `[cyan]` | cyan |
| `[blue]` | blue |
| `[purple]` | purple |
| `[pink]` | pink |
| `[alert]` | vermilion, the alert colour |
| `[black]` | black (useful on a background) |
| `[#ff8800]`, `[#f80]` | any hex colour; the `#` is optional |
| `[colour=pink]`, `[color=#0af]`, `[c=red]` | the same, spelled out |

```
[red]Colour [orange]in [yellow]every [green]tag [cyan]and [#ff8fd0]hex[/]
```

Because the `#` is optional, a bracketed word made of three or six hex digits is read as a colour:
`[add]`, `[bee]` and `[cafe00]` change the colour rather than appearing as text.

### Backgrounds

`[bg=COLOUR]` colours the unlit LEDs behind the following text, from palette names or hex.

```
[bg=blue][white] ON AIR [/][/]   [bg=#400][yellow] LIVE [/][/]
```

### Effects

Effects combine; each tag adds one.

| Tag | Effect |
|---|---|
| `[rainbow]` | colour cycles along the text and over time |
| `[blink]` | switches on and off |
| `[pulse]` | brightness swells and fades |
| `[wave]` | letters ripple up and down |
| `[shimmer]` | a bright band runs through the text |
| `[plain]` or `[noeffect]` | clears all effects |

```
[wave][shimmer][cyan]Incoming[/][/][/]   [blink][red]waiting[/][/]
```

Emoji take blink, pulse, wave and shimmer but not rainbow. Sprites take blink and wave.

### Sizes

Sizes are in LED rows; the panel is 66 rows high.

| Tag | Size |
|---|---|
| `[big]` | 58 |
| (default) | 44, or `@size` |
| `[small]` | 26 |
| `[tiny]` | 14 |
| `[size=30]`, `[s=30]` | any whole number, clamped to 8–66 |

```
[big]BIG[/] [small]and small[/] [tiny]and tiny[/]
```

### Fonts

Shortcut tags:

| Tag | Font on macOS |
|---|---|
| `[sans]` | Helvetica Bold (default) |
| `[serif]` | Georgia Bold |
| `[mono]` | Menlo Bold |
| `[script]` | Snell Roundhand |
| `[condensed]` | Impact |
| `[rounded]` | Arial Rounded Bold |
| `[typewriter]` | American Typewriter |
| `[chalk]` | Chalkduster |
| `[stencil]` | Copperplate |
| `[didot]` | Didot |
| `[futura]` | Futura |
| `[marker]` | Marker Felt |
| `[hand]` | Bradley Hand Bold |
| `[poster]` | Phosphate |
| `[pixel]` | a built-in bitmap font, scaled up |

Any installed font works with `[font=Name]` (or `[f=Name]`). Names are matched loosely: case,
spaces and punctuation are ignored, and a bold face is preferred, so `[font=HelveticaNeue]`,
`[font=helvetica-neue]` and `[font=Avenir]` all work. Because tags cannot contain spaces, write
multi-word names without them. Marquee looks in `/System/Library/Fonts`,
`/System/Library/Fonts/Supplemental`, `/Library/Fonts` and `~/Library/Fonts`. A path to a `.ttf`,
`.otf`, `.ttc`, `.otc` or `.dfont` file also works, including `~/…`.

The CJK shortcut is the exception: `cjk` (Arial Unicode) is listed by `GET /v1/fonts` but the board
does not accept it as a bare `[cjk]` tag. Use `[font=cjk]` or `@font=cjk`.

`GET /v1/fonts` lists the shortcuts and every installed font name. A font that cannot be found
falls back to sans, and the API warns about it.

```
[serif]Serif [mono]Mono [script]Script [condensed]Impact [font=Avenir]Avenir [pixel]Pixel
```

## Sprites

Animated pixel-art sprites, written as `:name:`:

| Sprite | |
|---|---|
| `:heart:` | heart |
| `:fire:` | flame |
| `:star:` | star |
| `:ufo:` | UFO, bobbing |
| `:wave:` | rolling water wave |
| `:check:` | green tick |
| `:cross:` | red cross |

An unknown `:word:` is shown as text, with a warning.

## Emoji

Type emoji directly: `🚀 🎉 ☕️ 👍🏽 🇬🇧`. Skin tones, flags and joined sequences work. They are drawn
in colour at the current text size, from Apple Color Emoji on macOS (Twemoji elsewhere). An emoji
that cannot be drawn appears as `?`.

## Live tokens

Tokens in curly braces are replaced as the board is drawn. Unknown tokens are left as they are.

| Token | Value |
|---|---|
| `{time}` | `14:05` |
| `{date}` | `07 Nov 2026` |
| `{day}` | `Saturday` |
| `{cpu}` | CPU use in per cent, e.g. `37` |
| `{mem}` | memory use in per cent, e.g. `64` |
| `{weather}` | `☁️ London 11°C overcast`, for `@location` |
| `{temp}` | `11°C`, for `@location` |
| `{incidents}` | PagerDuty summary: `2 triggered · 1 acknowledged`, `no open incidents` or `PagerDuty not set up` |
| `{incident}` | title of the newest triggered (else acknowledged) PagerDuty incident, or empty |

Weather comes from [Open-Meteo](https://open-meteo.com) (no key) and is cached for 15 minutes.
Until `@location` is set, `{weather}` reads `set @location=City`. PagerDuty setup is in
[GUIDE.md](GUIDE.md#pagerduty).

```
CPU [yellow]{cpu}%[/]  memory [cyan]{mem}%[/]   {weather}   [alert]{incidents}[/]
```

## Alerts

A line starting with `!` is an alert: the whole line is vermilion and pulses. Tags inside it still
apply.

```
! Deploy blocked :cross:
!Alerts pulse in vermilion
```

For an alert that interrupts whatever is playing, use a flash (`marquee flash`, `POST /v1/flash`)
instead.

## Settings

A line starting with `@` sets something for the whole board. Put settings anywhere; if one appears
twice the last wins.

| Setting | Default | |
|---|---|---|
| `@step=N` | 2 | Scroll speed: LEDs moved per frame, 1–40 |
| `@gap=N` | 34 | LEDs of space between messages |
| `@font=Name` | `sans` | Default font: a shortcut, installed font name (spaces allowed here) or file path |
| `@size=N` | 44 | Default size in LED rows, 8–66 |
| `@colour=C` (or `@color`) | warm white | Default text colour |
| `@dim=C` | `#1e1e1a` | Colour of unlit LEDs |
| `@bg=C` | `#0c0d0b` | Colour between the LEDs |
| `@theme=green` or `@theme=amber` | none | Phosphor look, below |
| `@location=City` | none | Place for `{weather}` and `{temp}` |

The `BOARD_STEP` environment variable changes the default step for a server started from a
shell.

### Themes

`@theme=green` and `@theme=amber` turn the board into a monochrome CRT: each LED keeps its
brightness but takes the phosphor colour, with a soft glow around lit LEDs and dimmer alternate
rows like scanlines. Other theme values are ignored, with a warning.

```
@theme=amber
@step=3
[big]Phosphor themes
green or amber, with glow and scanlines {time}
```

## Warnings

`PUT /v1/messages` and `POST /v1/messages` (and the MCP `say` and `set_messages` tools) return
warnings with line numbers for unknown tags, bad colours, non-numeric sizes, unknown sprites,
fonts that are not installed, unknown settings and unknown themes. The board still plays; the
problem text is shown as text or the default is used.

## A complete example

```
# Office board
@location=Manchester
@step=3
@gap=40
[white]{time}[/]  [cyan]{day}[/] {date}   {weather}
[rainbow]Welcome to the board[/] :star:
[serif]Fonts:[/] [script]script[/] [typewriter]typewriter[/] [condensed]CONDENSED[/] [pixel]PIXEL[/]
Coffee time ☕️ :heart: :heart:
CPU [yellow]{cpu}%[/]  memory [cyan]{mem}%[/]
[wave][green]Build passed[/][/] :check:   Deploy [blink][red]waiting[/][/] :fire:
[bg=blue][white] ON AIR [/][/]  [big]BIG[/] [small]and small[/]
On call: [alert]{incidents}[/] {incident}
! Fire drill at 11:00 :cross:
```
