package protocol

import (
	"context"
	"testing"
)

func TestHandshakeCandidateBoundary(t *testing.T) {
	for _, domain := range handshakeDomains {
		if err := ValidateHandshakeDomain(domain); err != nil {
			t.Fatalf("candidate %s: %v", domain, err)
		}
	}
	for _, domain := range []string{"www.apple.com", "https://www.kernel.org", "127.0.0.1", "bad\nname.org", "a..org", "-bad.org"} {
		if ValidateHandshakeDomain(domain) == nil {
			t.Fatalf("accepted %q", domain)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := SelectHandshakeDomain(ctx, ""); err == nil {
		t.Fatal("cancelled selection succeeded")
	}
	if _, err := SelectHandshakeDomain(ctx, "www.kernel.org"); err == nil {
		t.Fatal("cancelled explicit handshake succeeded")
	}
}
