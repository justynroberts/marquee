package api

import (
	"os"

	"github.com/grandcat/zeroconf"
)

// Advertise registers _marquee._tcp on the local network so apps can discover the panel.
// Returns a func that withdraws the advertisement.
func Advertise(port int, version string) (func(), error) {
	host, _ := os.Hostname()
	srv, err := zeroconf.Register("Marquee on "+host, "_marquee._tcp", "local.", port,
		[]string{"version=" + version, "api=/v1", "auth=bearer", "docs=/v1/openapi.json"}, nil)
	if err != nil {
		return func() {}, err
	}
	return srv.Shutdown, nil
}
