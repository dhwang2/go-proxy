package application

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"go-proxy/internal/config"
)

func BenchmarkCapacity(b *testing.B) {
	dir := os.Getenv("GPROXY_BENCH_FIXTURE")
	if dir == "" {
		b.Skip("set GPROXY_BENCH_FIXTURE to an isolated synthetic fixture")
	}
	paths := []*string{&config.SingBoxConfig, &config.UserMetaFile, &config.UserRouteFile, &config.UserTemplateFile, &config.FirewallConfigFile, &config.SnellConfigFile}
	names := []string{"conf/sing-box.json", "user-management.json", "user-route-rules.json", "user-route-templates.json", "firewall-ports.json", "snell-v6.conf"}
	previous := make([]string, len(paths))
	for i, path := range paths {
		previous[i] = *path
		*path = filepath.Join(dir, "root", names[i])
	}
	b.Cleanup(func() {
		for i, path := range paths {
			*path = previous[i]
		}
	})
	a := New(nil)
	a.RequireRoot = false
	a.LockDir = filepath.Join(dir, "locks")
	for _, name := range []string{"snapshot", "routing", "config", "sub-json"} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var err error
				var result Result
				switch name {
				case "snapshot":
					_, err = a.Snapshot(context.Background())
				case "routing":
					result, err = a.RoutingList(context.Background(), "")
				case "config":
					result, err = a.ConfigView(context.Background(), "sing-box", false)
				case "sub-json":
					result, err = a.Subscription(context.Background(), SubscriptionOptions{Target: "192.0.2.1", JSON: true})
				}
				if err != nil {
					b.Fatal(err)
				}
				if name != "snapshot" {
					if err := json.NewEncoder(io.Discard).Encode(result.Data); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
