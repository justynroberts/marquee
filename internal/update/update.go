// Package update checks GitHub Releases for a newer Marquee. It never installs anything by
// itself: the app offers the signed, notarised disk image and the person opens it.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Repo is the GitHub repository releases are published to.
const Repo = "justynroberts/marquee"

// APIBase is the GitHub API root (tests point it elsewhere).
var APIBase = "https://api.github.com"

// Release is the latest published release.
type Release struct {
	Version  string `json:"version"`  // without the leading v
	PageURL  string `json:"pageUrl"`  // release notes
	AssetURL string `json:"assetUrl"` // the .dmg, if attached
}

// Result of a check.
type Result struct {
	Current   string   `json:"current"`
	Latest    *Release `json:"latest,omitempty"`
	Available bool     `json:"available"`
	Checked   string   `json:"checked"`
	Error     string   `json:"error,omitempty"`
}

var client = &http.Client{Timeout: 15 * time.Second}

// Latest fetches the newest non-draft, non-prerelease release.
func Latest(ctx context.Context) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, APIBase+"/repos/"+Repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Marquee-update-check")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no releases published yet")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub: HTTP %d", resp.StatusCode)
	}
	var body struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		return nil, err
	}
	r := &Release{Version: strings.TrimPrefix(body.TagName, "v"), PageURL: body.HTMLURL}
	for _, a := range body.Assets {
		if strings.HasSuffix(strings.ToLower(a.Name), ".dmg") {
			r.AssetURL = a.URL
			break
		}
	}
	// Only follow links that stay on GitHub.
	if !strings.HasPrefix(r.PageURL, "https://github.com/") {
		r.PageURL = "https://github.com/" + Repo + "/releases/latest"
	}
	if r.AssetURL != "" && !strings.HasPrefix(r.AssetURL, "https://github.com/") {
		r.AssetURL = ""
	}
	return r, nil
}

// Check compares the latest release with current.
func Check(ctx context.Context, current string) Result {
	res := Result{Current: current, Checked: time.Now().UTC().Format(time.RFC3339)}
	r, err := Latest(ctx)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Latest = r
	res.Available = Newer(r.Version, current)
	return res
}

// Newer reports whether version a is newer than b (dotted numbers; a pre-release suffix such
// as -rc1 sorts before the release). A development build ("dev", "0.0.0-dev") is never offered
// an update.
func Newer(a, b string) bool {
	if b == "" || b == "dev" || strings.HasSuffix(b, "-dev") {
		return false
	}
	pa, sa := split(a)
	pb, sb := split(b)
	for i := 0; i < max(len(pa), len(pb)); i++ {
		x, y := at(pa, i), at(pb, i)
		if x != y {
			return x > y
		}
	}
	// Same numbers: a release beats a pre-release of it.
	return sa == "" && sb != ""
}

func split(v string) ([]int, string) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	suffix := ""
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v, suffix = v[:i], v[i:]
	}
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			n = 0
		}
		out = append(out, n)
	}
	return out, suffix
}

func at(v []int, i int) int {
	if i < len(v) {
		return v[i]
	}
	return 0
}
