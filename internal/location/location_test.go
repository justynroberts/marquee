package location

import "testing"

func TestPlaceRounds(t *testing.T) {
	if got := (Fix{Lat: 51.23456, Lon: -0.57321, Name: "Guildford"}).Place(); got != "@51.23,-0.57|Guildford" {
		t.Fatal(got)
	}
}
