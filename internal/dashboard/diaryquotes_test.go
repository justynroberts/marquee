package dashboard

import (
	"testing"
	"time"
)

// Every quotation must fit in three lines, leaving the fourth rule for the signature.
func TestDiaryQuotesFit(t *testing.T) {
	UseFont("hand")
	defer UseFont("")
	for _, q := range dyQuotes {
		if n := len(wrapWords("“"+q.text+"”", dyBodyPx, float64(W)-90-dyMargin, "hand")); n > 3 {
			t.Errorf("%q wraps to %d lines", q.text, n)
		}
	}
}

func TestDiaryDaylight(t *testing.T) {
	d := NewDiary(func() string { return "@53.0,-1.0|Test" })
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	var en *dyEntry
	for i := 0; i < 50 && en == nil; i++ {
		en = d.eDaylight(now)
		time.Sleep(10 * time.Millisecond)
	}
	if en == nil || len(en.lines) != 3 {
		t.Fatalf("daylight entry: %+v", en)
	}
	t.Log(en.big, en.lines)
}
