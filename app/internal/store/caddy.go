package store

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"go-proxy/internal/config"
)

// DefaultCaddyPort is where caddy-sub serves its site until `cert port`
// moves it: out of the way, since its first job is issuing the certificate.
const DefaultCaddyPort = 18443

// CaddySite is what gproxy reads back from the Caddyfile it wrote: the port
// the site is served on and the ACME contact, which a regenerated Caddyfile
// keeps unless a new one is given.
type CaddySite struct {
	Port  int
	Email string
}

// ReadCaddySite reads the Caddyfile gproxy generated. found is false when
// there is none; a site line without a readable port is the default port.
func ReadCaddySite() (site CaddySite, found bool) {
	file, err := os.Open(config.CaddyFile)
	if err != nil {
		return CaddySite{}, false
	}
	defer file.Close()
	site.Port = DefaultCaddyPort
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if email, ok := strings.CutPrefix(line, "email "); ok {
			site.Email = strings.TrimSpace(email)
			continue
		}
		// The one site line: "a.example.org:443, b.example.org:443 {".
		address, ok := strings.CutSuffix(line, " {")
		if !ok || !strings.Contains(address, ".") {
			continue
		}
		address, _, _ = strings.Cut(address, ",")
		if i := strings.LastIndex(address, ":"); i >= 0 {
			if port, err := strconv.Atoi(address[i+1:]); err == nil && port > 0 && port <= 65535 {
				site.Port = port
			}
		}
	}
	return site, true
}
