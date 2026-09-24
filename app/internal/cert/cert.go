package cert

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"go-proxy/internal/config"
	"go-proxy/internal/core"
	"go-proxy/internal/network"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
	"go-proxy/pkg/fileutil"
)

var domainRe = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

const certIssueTimeout = 5 * time.Minute

// IsValidDomain checks if the string is a valid domain name (not an IP).
func IsValidDomain(domain string) bool {
	if domain == "" || len(domain) > 253 {
		return false
	}
	if net.ParseIP(domain) != nil {
		return false
	}
	return domainRe.MatchString(domain)
}

func IsValidEmail(email string) bool {
	if email == "" {
		return true
	}
	parsed, err := mail.ParseAddress(email)
	return err == nil && parsed.Address == email && !strings.ContainsAny(email, "\r\n")
}

// ReadDomain reads the stored domain from .domain file.
func ReadDomain() string {
	data, err := os.ReadFile(config.DomainFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// WriteDomain writes the domain to .domain file.
func WriteDomain(domain string) error {
	return os.WriteFile(config.DomainFile, []byte(domain+"\n"), 0644)
}

// GenerateCaddyfile creates a Caddyfile for TLS certificate issuance.
func GenerateCaddyfile(domain, email string) error {
	if !IsValidDomain(domain) {
		return fmt.Errorf("invalid certificate domain")
	}
	emailLine := ""
	if email != "" {
		if !IsValidEmail(email) {
			return fmt.Errorf("invalid certificate email")
		}
		emailLine = "email " + email
	}
	f, err := os.Open(config.SingBoxConfig)
	if err != nil {
		return fmt.Errorf("read certificate domains: %w", err)
	}
	var existing store.SingBoxConfig
	err = json.NewDecoder(f).Decode(&existing)
	f.Close()
	if err != nil {
		return fmt.Errorf("read certificate domains: %w", err)
	}
	domains := map[string]bool{strings.ToLower(domain): true}
	for _, inbound := range existing.Inbounds {
		if inbound.TLS == nil || !inbound.TLS.Enabled || inbound.HasReality() {
			continue
		}
		name := strings.ToLower(inbound.TLS.ServerName)
		if !IsValidDomain(name) {
			return fmt.Errorf("configured tls node has an invalid certificate domain")
		}
		domains[name] = true
	}
	sites := make([]string, 0, len(domains))
	for name := range domains {
		sites = append(sites, name+":18443")
	}
	sort.Strings(sites)
	content := fmt.Sprintf(`{
    %s
    auto_https disable_redirects
}

%s {
    tls {
        protocols tls1.2 tls1.3
    }
    respond "ok" 200
}
`, emailLine, strings.Join(sites, ", "))
	return fileutil.AtomicWrite(config.CaddyFile, []byte(content))
}

// CertExists checks if TLS certificate files exist for the given domain
// under any ACME issuer directory in CaddyCertDir.
func CertExists(domain string) bool {
	certFile, keyFile := findCertPair(domain)
	if certFile == "" || keyFile == "" {
		return false
	}
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil || len(pair.Certificate) == 0 {
		return false
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return false
	}
	return leaf.VerifyHostname(domain) == nil && time.Now().After(leaf.NotBefore) && time.Now().Before(leaf.NotAfter)
}

// WaitForCert waits for caddy to place the certificate, and reads caddy's log
// while it waits. A file that has not appeared says only that it has not;
// caddy already knows why, so a refusal that names the hour it lifts ends the
// wait immediately instead of spending a deadline on it, and a wait that does
// run out reports what the authority actually said.
func WaitForCert(ctx context.Context, domain string, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			if problem, found := lastIssuanceProblem(domain); found && problem.Reason != "" {
				return fmt.Errorf("certificate was not issued after %v: %s", timeout, problem.Reason)
			}
			return fmt.Errorf("certificate issuance timed out after %v; check domain dns records", timeout)
		case <-ticker.C:
			if CertExists(domain) {
				return nil
			}
			if problem, found := lastIssuanceProblem(domain); found && problem.Final {
				return fmt.Errorf("certificate was refused: %s", problem.Reason)
			}
		}
	}
}

// RestartCaddySub restarts the caddy-sub systemd service.
func RestartCaddySub(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "systemctl", "restart", "caddy-sub")
	return cmd.Run()
}

func WaitForCaddySub(ctx context.Context) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(10 * time.Second)
	for {
		if exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", "caddy-sub").Run() == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("certificate service did not become active")
		case <-ticker.C:
		}
	}
}

func findCertPair(domain string) (string, string) {
	if domain == "" {
		return "", ""
	}

	var certFile string
	var keyFile string
	_ = filepath.WalkDir(config.CaddyCertDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if filepath.Base(path) != domain+".crt" || filepath.Base(filepath.Dir(path)) != domain {
			return nil
		}
		keyCandidate := strings.TrimSuffix(path, ".crt") + ".key"
		if _, err := os.Stat(keyCandidate); err != nil {
			return nil
		}
		certFile = path
		keyFile = keyCandidate
		return fs.SkipAll
	})
	return certFile, keyFile
}

func domainPointsToThisServer(ctx context.Context, domain string) bool {
	if domain == "" {
		return true
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupIPAddr(lookupCtx, domain)
	if err != nil || len(addrs) == 0 {
		return false
	}

	var localV4, localV6 string
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); localV4 = detectPublicCertIP(lookupCtx, "https://api.ipify.org") }()
	go func() { defer wg.Done(); localV6 = detectPublicCertIP(lookupCtx, "https://api6.ipify.org") }()
	wg.Wait()
	if localV4 == "" && localV6 == "" {
		return false
	}

	for _, addr := range addrs {
		ip := addr.IP
		switch ip.String() {
		case localV4, localV6:
			return true
		}
	}
	return false
}

func detectPublicCertIP(ctx context.Context, endpoints ...string) string {
	client := &http.Client{Timeout: 2 * time.Second}
	for _, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		if err != nil {
			continue
		}
		ip := net.ParseIP(strings.TrimSpace(string(body)))
		if ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() {
			return ip.String()
		}
	}
	return ""
}

func refreshManagedFirewall(ctx context.Context) error {
	managed, err := network.FirewallManaged(ctx)
	if err != nil {
		return err
	}
	if !managed {
		return nil
	}
	s, err := store.Load()
	if err != nil {
		return err
	}
	return network.ApplyFirewallConvergence(ctx, s)
}

type Status struct {
	Domain    string     `json:"domain"`
	Ready     bool       `json:"ready"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func Inspect() Status {
	domain := ReadDomain()
	result := Status{Domain: domain, Ready: CertExists(domain)}
	certFile, keyFile := findCertPair(domain)
	if certFile != "" && keyFile != "" {
		pair, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err == nil && len(pair.Certificate) > 0 {
			leaf, err := x509.ParseCertificate(pair.Certificate[0])
			if err == nil {
				result.ExpiresAt = &leaf.NotAfter
			}
		}
	}
	return result
}

func EnsureCertificateState(ctx context.Context, domain, email string, progress func(string), state func(func() error) error) error {
	if !IsValidDomain(domain) {
		return fmt.Errorf("invalid certificate domain")
	}
	if !IsValidEmail(email) {
		return fmt.Errorf("invalid certificate email")
	}
	if CertExists(domain) {
		return nil
	}
	if progress != nil {
		progress("checking certificate domain")
	}
	if !domainPointsToThisServer(ctx, domain) {
		return fmt.Errorf("certificate domain does not resolve to this server")
	}
	if progress != nil {
		progress("preparing caddy")
	}
	if err := core.Ensure(ctx, core.CompCaddy, ""); err != nil {
		return err
	}
	if err := state(func() error {
		if err := WriteDomain(domain); err != nil {
			return err
		}
		return GenerateCaddyfile(domain, email)
	}); err != nil {
		return err
	}
	if err := service.ProvisionCaddySub(ctx); err != nil {
		return err
	}
	if err := refreshManagedFirewall(ctx); err != nil {
		return err
	}
	if err := service.Enable(ctx, service.CaddySub); err != nil {
		return err
	}
	if err := RestartCaddySub(ctx); err != nil {
		return err
	}
	if err := WaitForCaddySub(ctx); err != nil {
		return err
	}
	if progress != nil {
		progress("waiting for certificate issuance")
	}
	return WaitForCert(ctx, domain, certIssueTimeout)
}
