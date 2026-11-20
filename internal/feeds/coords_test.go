package feeds

import "testing"

func TestGeocodeCoords(t *testing.T) {
	p, err := Geocode("@51.24,-0.57|Guildford")
	if err != nil || p.Name != "Guildford" || p.Latitude != 51.24 || p.Longitude != -0.57 {
		t.Fatalf("%+v %v", p, err)
	}
	if _, ok := parseCoords("London"); ok {
		t.Fatal("a name parsed as coordinates")
	}
}

func TestUKPostcode(t *testing.T) {
	for in, want := range map[string]string{"sw1a 1aa": "SW1A 1AA", "M11AE": "M1 1AE", " B33 8TH ": "B33 8TH"} {
		if got, ok := UKPostcode(in); !ok || got != want {
			t.Errorf("%q: %q %v", in, got, ok)
		}
	}
	for _, in := range []string{"London", "Long Eaton", "12345"} {
		if _, ok := UKPostcode(in); ok {
			t.Errorf("%q read as a postcode", in)
		}
	}
	if Outcode("SW1A 1AA") != "SW1A" {
		t.Error(Outcode("SW1A 1AA"))
	}
}
