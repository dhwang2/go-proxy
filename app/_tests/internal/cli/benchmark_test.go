package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"go-proxy/internal/config"
)

func BenchmarkCLIDataOutput(b *testing.B) {
	dir := os.Getenv("GPROXY_BENCH_FIXTURE")
	if dir == "" {
		b.Skip("requires a synthetic fixture")
	}
	paths := []*string{&config.SingBoxConfig, &config.UserMetaFile, &config.UserRouteFile, &config.UserTemplateFile, &config.FirewallConfigFile, &config.SnellConfigFile}
	names := []string{"conf/sing-box.json", "user-management.json", "user-route-rules.json", "user-route-templates.json", "firewall-ports.json", "snell-v6.conf"}
	old := make([]string, len(paths))
	for i, p := range paths {
		old[i] = *p
		*p = filepath.Join(dir, "root", names[i])
	}
	b.Cleanup(func() {
		for i, p := range paths {
			*p = old[i]
		}
	})
	for _, test := range []struct {
		name string
		args []string
	}{
		{"routing", []string{"route", "show"}}, {"routing-json", []string{"route", "show", "--json"}}, {"sub-json", []string{"sub", "--target", "192.0.2.1", "--json"}},
	} {
		b.Run(test.name, func(b *testing.B) {
			out, err := os.CreateTemp(b.TempDir(), "output-")
			if err != nil {
				b.Fatal(err)
			}
			defer out.Close()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				r := New("bench", "", nil, out, io.Discard)
				r.App.LockDir = filepath.Join(dir, "locks")
				if code := r.Run(context.Background(), test.args); code != 0 {
					b.Fatalf("exit %d", code)
				}
			}
		})
	}
}
