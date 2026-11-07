package emoji

import "testing"

func TestRender(t *testing.T) {
	for _, e := range []string{"🚀", "☕️", "🎉", "👍🏽", "🇬🇧", "👨‍👩‍👧"} {
		s, ok := Render(e, 30)
		if !ok {
			t.Errorf("%q not found", e)
			continue
		}
		lit := 0
		for _, l := range s.Lit {
			if l {
				lit++
			}
		}
		if s.H != 30 || lit < 50 {
			t.Errorf("%q: %dx%d lit=%d", e, s.W, s.H, lit)
		}
	}
	if _, ok := Render("A", 30); ok {
		t.Error("plain letter should not render")
	}
}

func TestSplit(t *testing.T) {
	got := Split("Hi 🚀🎉 ok ☕️!")
	want := []Run{{"Hi ", false}, {"🚀", true}, {"🎉", true}, {" ok ", false}, {"☕️", true}, {"!", false}}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("run %d: got %v want %v", i, got[i], want[i])
		}
	}
}
