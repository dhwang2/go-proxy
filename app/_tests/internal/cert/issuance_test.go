package cert

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-proxy/internal/config"
)

const rateLimited = `{"level":"error","ts":1790068525.27,"logger":"tls.obtain","msg":"could not get certificate from issuer","identifier":"www.example.org","issuer":"acme-v02.api.letsencrypt.org-directory","error":"HTTP 429 urn:ietf:params:acme:error:rateLimited - too many certificates (5) already issued for this exact set of identifiers in the last 168h0m0s, retry after 2026-09-22 14:19:12 UTC: see https://letsencrypt.org/docs/rate-limits/"}`

func caddyLog(t *testing.T, lines ...string) {
	t.Helper()
	saved := config.CaddySubLog
	config.CaddySubLog = filepath.Join(t.TempDir(), "caddy-sub.service.log")
	t.Cleanup(func() { config.CaddySubLog = saved })
	if err := os.WriteFile(config.CaddySubLog, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

// A refusal that names the hour it lifts is proof this wait cannot succeed.
// Spending the whole deadline on it told the reader "timed out, check dns",
// which is the wrong thing to go and check.
func TestRateLimitEndsTheWaitWithTheAuthoritysReason(t *testing.T) {
	caddyLog(t, `{"level":"info","logger":"http.acme_client","msg":"trying to solve challenge"}`, rateLimited)

	problem, found := lastIssuanceProblem("www.example.org")
	if !found || !problem.Final {
		t.Fatalf("a rate limit was not read as final: %+v %t", problem, found)
	}
	for _, want := range []string{"too many certificates (5)", "retry after 2026-09-22 14:19:12 UTC"} {
		if !strings.Contains(problem.Reason, want) {
			t.Fatalf("the reason does not carry %q: %q", want, problem.Reason)
		}
	}
	// The transport and the urn are this program's noise, not the reader's.
	if strings.Contains(problem.Reason, "HTTP 429") || strings.Contains(problem.Reason, "urn:ietf") {
		t.Fatalf("the reason still carries protocol noise: %q", problem.Reason)
	}

	// And the wait itself ends on it rather than on its deadline.
	started := time.Now()
	err := WaitForCert(context.Background(), "www.example.org", time.Minute)
	if err == nil || !strings.Contains(err.Error(), "too many certificates") {
		t.Fatalf("the wait did not report the refusal: %v", err)
	}
	if waited := time.Since(started); waited > 20*time.Second {
		t.Fatalf("the wait spent %v on a refusal that named its own retry time", waited)
	}
}

// An error caddy will retry is not final: the wait continues, and if it does
// run out the reader is told what the authority said rather than to check DNS.
func TestARetryableFailureIsReportedOnlyWhenTheWaitRunsOut(t *testing.T) {
	caddyLog(t, `{"level":"error","logger":"tls.obtain","msg":"could not get certificate from issuer","identifier":"www.example.org","error":"[www.example.org] solving challenge: presenting for challenge: could not connect (ca=https://acme-v02.api.letsencrypt.org/directory)"}`)

	problem, found := lastIssuanceProblem("www.example.org")
	if !found || problem.Final {
		t.Fatalf("a retryable failure was read as final: %+v %t", problem, found)
	}
	if strings.Contains(problem.Reason, "ca=") {
		t.Fatalf("the reason kept caddy's bookkeeping: %q", problem.Reason)
	}
	err := WaitForCert(context.Background(), "www.example.org", 3*time.Second)
	if err == nil || !strings.Contains(err.Error(), "solving challenge") {
		t.Fatalf("the timeout did not carry the reason: %v", err)
	}
}

// Another name's failure is not this one's, and a log that says nothing leaves
// the original message in place.
func TestIssuanceProblemsAreMatchedAndOptional(t *testing.T) {
	caddyLog(t, strings.Replace(rateLimited, "www.example.org", "other.example.org", 1))
	if _, found := lastIssuanceProblem("www.example.org"); found {
		t.Fatal("another domain's refusal was read as this one's")
	}
	caddyLog(t, `{"level":"info","logger":"tls","msg":"served key authentication certificate"}`)
	if _, found := lastIssuanceProblem("www.example.org"); found {
		t.Fatal("an informational log was read as a failure")
	}
	saved := config.CaddySubLog
	config.CaddySubLog = filepath.Join(t.TempDir(), "absent.log")
	t.Cleanup(func() { config.CaddySubLog = saved })
	err := WaitForCert(context.Background(), "www.example.org", time.Second)
	if err == nil || !strings.Contains(err.Error(), "check domain dns records") {
		t.Fatalf("with no log to read, the original message should stand: %v", err)
	}
}
