package protocol

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"
)

var handshakeDomains = []string{
	"www.kernel.org", "www.freebsd.org",
	"www.openbsd.org", "www.rust-lang.org", "www.postgresql.org",
}

func ValidateHandshakeDomain(domain string) error {
	if len(domain) > 253 || net.ParseIP(domain) != nil || !strings.Contains(domain, ".") || strings.EqualFold(domain, "www.apple.com") {
		return fmt.Errorf("invalid handshake domain")
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("invalid handshake domain")
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return fmt.Errorf("invalid handshake domain")
			}
		}
	}
	return nil
}

func SelectHandshakeDomain(ctx context.Context, explicit string) (string, error) {
	if explicit != "" {
		if err := ValidateHandshakeDomain(explicit); err != nil {
			return "", err
		}
		if err := checkHandshakeDomain(ctx, explicit); err != nil {
			return "", fmt.Errorf("handshake domain %q failed tls 1.3 certificate verification: %w", explicit, err)
		}
		return explicit, nil
	}
	candidates := append([]string(nil), handshakeDomains...)
	for i := len(candidates) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return "", fmt.Errorf("select handshake domain: %w", err)
		}
		j := int(n.Int64())
		candidates[i], candidates[j] = candidates[j], candidates[i]
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	for _, domain := range candidates {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("select handshake domain: %w", err)
		}
		if checkHandshakeDomain(ctx, domain) == nil {
			return domain, nil
		}
	}
	return "", fmt.Errorf("no candidate passed tls 1.3 certificate verification; specify a reachable handshake domain")
}

func checkHandshakeDomain(ctx context.Context, domain string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	dialer := tls.Dialer{Config: &tls.Config{
		ServerName: domain, MinVersion: tls.VersionTLS13,
		NextProtos: []string{"h2", "http/1.1"},
	}}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(domain, "443"))
	if err != nil {
		return err
	}
	return conn.Close()
}
