package update

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go-proxy/pkg/sysutil"
)

type redirectedTransport struct{ target *url.URL }

func (r redirectedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	copy.URL.Scheme = r.target.Scheme
	copy.URL.Host = r.target.Host
	return http.DefaultTransport.RoundTrip(copy)
}

func TestLatestUpdateNeverDowngradesDevelopmentBuild(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v0.1.59","assets":[{"name":"gproxy-linux-%s","browser_download_url":"https://example.com/binary","digest":"sha256:%s"}]}`, sysutil.Arch(), strings.Repeat("a", 64))
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	previous := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: redirectedTransport{target: target}}
	t.Cleanup(func() { http.DefaultClient = previous })
	for _, test := range []struct {
		current, requested string
		available          bool
	}{
		{"v0.1.58", "", true}, {"v0.1.59", "", false}, {"v0.2.0-dev", "", false}, {"dev", "", false}, {"v0.2.0-dev", "0.1.59", true},
	} {
		result, err := ResolveSelfUpdate(context.Background(), test.current, test.requested)
		if err != nil {
			t.Fatal(err)
		}
		if result.UpdateAvail != test.available {
			t.Fatalf("current=%s requested=%s availability=%t", test.current, test.requested, result.UpdateAvail)
		}
	}
}
