package update

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.2.0", "0.1.0", true}, {"0.1.0", "0.1.0", false}, {"0.1.0", "0.2.0", false},
		{"1.0", "0.9.9", true}, {"0.10.0", "0.9.0", true}, {"0.2.0", "0.2.0-rc1", true},
		{"0.2.0-rc1", "0.2.0", false}, {"9.9.9", "dev", false}, {"9.9.9", "0.0.0-dev", false},
		{"v0.3.0", "0.2.9", true},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/"+Repo+"/releases/latest" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"tag_name":"v0.3.0","html_url":"https://github.com/justynroberts/marquee/releases/tag/v0.3.0",
		  "assets":[{"name":"notes.txt","browser_download_url":"https://github.com/x/notes.txt"},
		            {"name":"Marquee-0.3.0.dmg","browser_download_url":"https://github.com/justynroberts/marquee/releases/download/v0.3.0/Marquee-0.3.0.dmg"}]}`)
	}))
	defer srv.Close()
	old := APIBase
	APIBase = srv.URL
	defer func() { APIBase = old }()
	r := Check(context.Background(), "0.1.0")
	if !r.Available || r.Latest.Version != "0.3.0" || r.Latest.AssetURL == "" || r.Error != "" {
		t.Fatalf("%+v %+v", r, r.Latest)
	}
	if r := Check(context.Background(), "0.3.0"); r.Available {
		t.Fatal("same version offered")
	}
}

func TestOffSiteLinksDropped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v9.0.0","html_url":"https://evil.example/","assets":[{"name":"M.dmg","browser_download_url":"https://evil.example/M.dmg"}]}`)
	}))
	defer srv.Close()
	old := APIBase
	APIBase = srv.URL
	defer func() { APIBase = old }()
	r, err := Latest(context.Background())
	if err != nil || r.AssetURL != "" || r.PageURL != "https://github.com/"+Repo+"/releases/latest" {
		t.Fatalf("%v %+v", err, r)
	}
}
