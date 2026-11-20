package dashboard

import (
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// The diary's thoughts: short lines from writers long out of copyright, copied into the page and
// signed off in red. Kept in the binary so they need no network and never fail.

type dyQuoteT struct{ text, who string }

var dyQuotes = []dyQuoteT{
	{"Well done is better than well said.", "Benjamin Franklin"},
	{"Lost time is never found again.", "Benjamin Franklin"},
	{"Little strokes fell great oaks.", "Benjamin Franklin"},
	{"Dost thou love life? Then do not squander time, for that's the stuff life is made of.", "Benjamin Franklin"},
	{"The unexamined life is not worth living.", "Socrates"},
	{"Hope is the thing with feathers that perches in the soul.", "Emily Dickinson"},
	{"I dwell in Possibility.", "Emily Dickinson"},
	{"Forever is composed of nows.", "Emily Dickinson"},
	{"There is nothing either good or bad, but thinking makes it so.", "Shakespeare"},
	{"We know what we are, but know not what we may be.", "Shakespeare"},
	{"Be not afraid of greatness.", "Shakespeare"},
	{"The fault, dear Brutus, is not in our stars, but in ourselves.", "Shakespeare"},
	{"Our doubts are traitors, and make us lose the good we oft might win by fearing to attempt.", "Shakespeare"},
	{"The best way out is always through.", "Robert Frost"},
	{"The woods are lovely, dark and deep, but I have promises to keep, and miles to go before I sleep.", "Robert Frost"},
	{"Two roads diverged in a wood, and I took the one less travelled by, and that has made all the difference.", "Robert Frost"},
	{"Fortune favours the bold.", "Virgil"},
	{"No man is an island, entire of itself.", "John Donne"},
	{"A journey of a thousand miles begins with a single step.", "Lao Tzu"},
	{"The mind is not a vessel to be filled, but a fire to be kindled.", "Plutarch"},
	{"Waste no more time arguing what a good man should be. Be one.", "Marcus Aurelius"},
	{"The impediment to action advances action. What stands in the way becomes the way.", "Marcus Aurelius"},
	{"Dwell on the beauty of life. Watch the stars, and see yourself running with them.", "Marcus Aurelius"},
	{"While we are postponing, life speeds by.", "Seneca"},
	{"It is not that we have a short time to live, but that we waste a lot of it.", "Seneca"},
	{"Begin at once to live.", "Seneca"},
	{"To strive, to seek, to find, and not to yield.", "Tennyson"},
	{"'Tis better to have loved and lost than never to have loved at all.", "Tennyson"},
	{"Our life is frittered away by detail. Simplify, simplify.", "Thoreau"},
	{"Go confidently in the direction of your dreams.", "after Thoreau"},
	{"Nothing great was ever achieved without enthusiasm.", "Emerson"},
	{"Write it on your heart that every day is the best day in the year.", "Emerson"},
	{"The only way to have a friend is to be one.", "Emerson"},
	{"I am large, I contain multitudes.", "Walt Whitman"},
	{"Grow old along with me! The best is yet to be.", "Robert Browning"},
	{"A man's reach should exceed his grasp, or what's a heaven for?", "Robert Browning"},
	{"Hope springs eternal in the human breast.", "Alexander Pope"},
	{"To err is human; to forgive, divine.", "Alexander Pope"},
	{"A thing of beauty is a joy for ever.", "John Keats"},
	{"Heard melodies are sweet, but those unheard are sweeter.", "John Keats"},
	{"If Winter comes, can Spring be far behind?", "Shelley"},
	{"Tread softly because you tread on my dreams.", "W. B. Yeats"},
	{"Make haste slowly.", "Augustus"},
	{"Well begun is half done.", "Aristotle"},
	{"He who has a why to live can bear almost any how.", "Nietzsche"},
	{"One must still have chaos in oneself to give birth to a dancing star.", "Nietzsche"},
	{"Imagination is more important than knowledge.", "Einstein"},
	{"Life is like riding a bicycle. To keep your balance you must keep moving.", "Einstein"},
	{"Nothing in life is to be feared, it is only to be understood.", "Marie Curie"},
	{"Genius is one per cent inspiration and ninety-nine per cent perspiration.", "Edison"},
	{"Not to go back is somewhat to advance.", "Horace"},
	{"Seize the day, trusting as little as possible in tomorrow.", "Horace"},
	{"The beginning is the most important part of the work.", "Plato"},
	{"Happiness depends upon ourselves.", "Aristotle"},
}

// dyQuoteN steps through the quotes, starting somewhere new each day.
var dyQuoteN atomic.Int64

func (d *Diary) eQuote(now time.Time) *dyEntry {
	i := (now.YearDay()*7 + int(dyQuoteN.Add(1))) % len(dyQuotes)
	q := dyQuotes[i]
	return &dyEntry{title: "a thought", lines: []string{"“" + q.text + "”"}, sign: "— " + q.who}
}

// eDaylight: when the sun rose and sets, how long the day is, and whether the days are drawing
// in or stretching out.
func (d *Diary) eDaylight(now time.Time) *dyEntry {
	place := d.place()
	if place == "" {
		return nil
	}
	p, _, ok, _ := d.geo.get(place, func(s string) (*feeds.Place, error) {
		pl, err := feeds.Geocode(s)
		return &pl, err
	})
	if !ok || p == nil {
		return nil
	}
	today := SunTimes(now, p.Latitude, p.Longitude)
	yest := SunTimes(now.AddDate(0, 0, -1), p.Latitude, p.Longitude)
	day := today.Daylight()
	if day == 0 {
		return nil
	}
	h, m := int(day.Hours()), int(day.Minutes())%60
	en := &dyEntry{title: "the light", big: fmt.Sprintf("%dh%02d", h, m), caption: "of daylight"}
	rose := "rose"
	if now.Before(today.Rise) {
		rose = "rises"
	}
	sets := "sets"
	if now.After(today.Set) {
		sets = "set"
	}
	en.lines = append(en.lines, fmt.Sprintf("The sun %s at %s and %s at %s.", rose, clockHM(today.Rise), sets, clockHM(today.Set)))
	diff := (day - yest.Daylight()).Minutes()
	switch {
	case math.Abs(diff) < 0.5:
		en.lines = append(en.lines, "As long a day as yesterday, near enough.")
	case diff < 0:
		en.lines = append(en.lines, fmt.Sprintf("Drawing in, %s less than yesterday.", mins(-diff)))
	default:
		en.lines = append(en.lines, fmt.Sprintf("Stretching out, %s more than yesterday.", mins(diff)))
	}
	// the solstices, north of the equator
	y := now.Year()
	next := func(m time.Month) time.Time {
		t := time.Date(y, m, 21, 0, 0, 0, 0, now.Location())
		if !t.After(now) {
			t = t.AddDate(1, 0, 0)
		}
		return t
	}
	win, sum := next(time.December), next(time.June)
	if p.Latitude < 0 {
		win, sum = sum, win
	}
	if win.Before(sum) {
		en.lines = append(en.lines, fmt.Sprintf("The shortest day is %s away.", days(win.Sub(now).Hours()/24)))
	} else {
		en.lines = append(en.lines, fmt.Sprintf("The longest day is %s away.", days(sum.Sub(now).Hours()/24)))
	}
	return en
}

func mins(m float64) string {
	n := int(math.Round(m))
	if n == 1 {
		return "a minute"
	}
	return numWord(n) + " minutes"
}
