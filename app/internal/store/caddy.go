package store

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go-proxy/internal/config"
)

// DefaultCaddyPort is where caddy-sub serves its site when 443 was taken as
// Caddy was set up: out of the way, since its first job is issuing the
// certificate. `cert port` moves it.
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

// CaddyCertPair is where Caddy keeps the certificate and key for domain,
// under whichever ACME issuer issued it; empty when there is none.
func CaddyCertPair(domain string) (certFile, keyFile string) {
	if domain == "" {
		return "", ""
	}
	_ = filepath.WalkDir(config.CaddyCertDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Base(path) != domain+".crt" || filepath.Base(filepath.Dir(path)) != domain {
			return nil
		}
		key := strings.TrimSuffix(path, ".crt") + ".key"
		if _, err := os.Stat(key); err != nil {
			return nil
		}
		certFile, keyFile = path, key
		return fs.SkipAll
	})
	return certFile, keyFile
}
