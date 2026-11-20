# Design

- Panel: near-black ground (12,13,11), paper ink (230,226,214), one vermilion accent (255,91,56)
  used only for alerts and hot readings. Scenes and the board render on a 275x66 grid of round
  LEDs; the dashboard renders at native 1920x462 so small text stays sharp.
- Settings page (`web/settings.html`, `/settings`): the remote's Terminal-editorial tokens; an index
  column of settings with set/default state, then one row per setting (title, file and the screens
  it feeds on the left; help, field and buttons on the right). The recognisable idea: each field
  answers back on the line beneath it, in phosphor green, with what Marquee understood
  (`→ Westminster 51.50, -0.14 · grid region SW1A`), or the reason in vermilion.
- Info screens (clocks, news, weather, flights, quakes, markets, air, calendar...; `palette.go`):
  the LEDs swallow dark greys, so these use brighter text greys and colour for meaning: cyan for
  headings and names, amber for the key number, green for good or rising, violet for a second
  series, vermilion still for alerts and the worst readings. Scales run green > amber >
  vermilion. Text is SF Mono Bold, since regular strokes look thin on the LEDs, with a
  few screens in another system face for variety (`programFonts` in dashboard.go): DIN Alternate
  Bold for the number-led ones (clocks, progress, pomodoro, fx, markets, countdown, netwatch,
  claudemeter), Georgia Bold for news and history, Rockwell Bold for weather, rain, sun and air,
  Avenir Next Bold for the playful Claude screens,
  and the diary screens (claudediary, diary), the one place that breaks the dark ground: a candle-lit
  parchment page, Homemade Apple handwriting in iron-gall ink (blue-black drying to sepia), Meddon
  for the date and big figures in red, a quill whose nib follows the ink, and a noise field the ink
  soaks away by. Both fonts are embedded (internal/dashboard/fonts, with licences). A string with a character the face lacks
  falls back to SF Mono Bold. The Claude screens
  add Claude's terracotta and a hand-drawn spark. The system dashboard keeps the original
  palette and regular weight for Pillow parity.
- Web remote: Terminal-editorial (carried over from thermalright-display): near-black, one mono
  typeface, phosphor-green accent, no cards or shadows, FintonLabs info button, reduced motion.
- Menu bar: native macOS menu, template icon (dot-matrix M). App icon: vermilion dot-matrix M on
  a near-black field.
