package engine

import (
	"testing"

	"github.com/justynroberts/marquee/internal/feeds"
)

func TestInterleave(t *testing.T) {
	a := []feeds.Headline{{Title: "a1"}, {Title: "a2"}, {Title: "a3"}}
	b := []feeds.Headline{{Title: "b1"}}
	got := ""
	for _, h := range interleave([][]feeds.Headline{a, b}) {
		got += h.Title + " "
	}
	if got != "a1 b1 a2 a3 " {
		t.Fatal(got)
	}
}
