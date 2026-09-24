package cert

import (
	"encoding/json"
	"io"
	"os"
	"strings"

	"go-proxy/internal/config"
	"go-proxy/pkg/textutil"
)

// issuanceTail is how much of caddy's log is read looking for the reason a
// certificate has not arrived. The interesting entry is always the most recent
// one, and the log is written by another program: it is read bounded or not at
// all.
const issuanceTail = 64 << 10

// issuanceProblem is what caddy last said about obtaining this certificate.
// Final means waiting cannot help: caddy names the time the limit lifts, and it
// is not within any deadline this command has.
type issuanceProblem struct {
	Reason string
	Final  bool
}

// lastIssuanceProblem reads caddy's log for the newest failure to obtain the
// certificate for domain. Waiting for a file to appear says only that it has
// not; caddy already knows why, and answering with its reason is the difference
// between "timed out, check dns" and "the weekly limit for this name is spent
// until 14:19 UTC".
func lastIssuanceProblem(domain string) (issuanceProblem, bool) {
	data, err := tailFile(config.CaddySubLog, issuanceTail)
	if err != nil {
		return issuanceProblem{}, false
	}
	lines := strings.Split(string(data), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var entry struct {
			Level      string `json:"level"`
			Logger     string `json:"logger"`
			Message    string `json:"msg"`
			Identifier string `json:"identifier"`
			Error      string `json:"error"`
		}
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		if entry.Level != "error" || entry.Error == "" {
			continue
		}
		if !strings.HasPrefix(entry.Logger, "tls.obtain") && !strings.Contains(entry.Message, "certificate") {
			continue
		}
		// An entry that names an identifier names which one; one that does not
		// belongs to the only issuance this program runs.
		if entry.Identifier != "" && entry.Identifier != domain {
			continue
		}
		return issuanceProblem{Reason: issuanceReason(entry.Error), Final: isRateLimited(entry.Error)}, true
	}
	return issuanceProblem{}, false
}

// isRateLimited reports whether the certificate authority refused because the
// name has had too many certificates. That refusal carries the time it lifts,
// which is hours away, so retrying inside this command's deadline cannot work.
func isRateLimited(reason string) bool {
	lowered := strings.ToLower(reason)
	return strings.Contains(lowered, "ratelimited") || strings.Contains(lowered, "too many certificates")
}

// issuanceReason trims the parts of caddy's error that repeat what the caller
// already knows -- the transport, the urn, the retry bookkeeping -- and keeps
// the sentence a person acts on, including the documentation link when the
// authority supplied one.
func issuanceReason(reason string) string {
	reason = textutil.Clean(strings.TrimSpace(reason))
	if _, after, found := strings.Cut(reason, "urn:ietf:params:acme:error:"); found {
		if _, sentence, ok := strings.Cut(after, " - "); ok {
			reason = sentence
		}
	}
	// caddy appends its own attempt counter and the ca it was talking to; the
	// authority's own words end at the first of those.
	for _, suffix := range []string{" (ca=", "\n"} {
		if index := strings.Index(reason, suffix); index > 0 {
			reason = reason[:index]
		}
	}
	return strings.TrimSpace(reason)
}

// tailFile reads at most limit bytes from the end of path.
func tailFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		if _, err := file.Seek(-limit, io.SeekEnd); err != nil {
			return nil, err
		}
	}
	return io.ReadAll(io.LimitReader(file, limit))
}
