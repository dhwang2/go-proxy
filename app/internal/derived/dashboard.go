package derived

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dhwang2/go-proxy/internal/store"
)

func InstalledProtocols(config *store.SingboxConfig) []string {
	if config == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(config.Inbounds))
	out := make([]string, 0, len(config.Inbounds))
	for _, in := range config.Inbounds {
		p := normalizeProto(in.Type)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{"none"}
	}
	sort.Strings(out)
	return out
}

func ListeningPorts(config *store.SingboxConfig) []int {
	if config == nil {
		return nil
	}
	seen := make(map[int]struct{}, len(config.Inbounds))
	out := make([]int, 0, len(config.Inbounds))
	for _, in := range config.Inbounds {
		if in.ListenPort <= 0 {
			continue
		}
		if _, ok := seen[in.ListenPort]; ok {
			continue
		}
		seen[in.ListenPort] = struct{}{}
		out = append(out, in.ListenPort)
	}
	sort.Ints(out)
	return out
}

func FormatPorts(ports []int) string {
	if len(ports) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		parts = append(parts, strconv.Itoa(p))
	}
	return strings.Join(parts, ",")
}

func SummaryLine(config *store.SingboxConfig) string {
	return fmt.Sprintf("protocols=%s ports=%s rules=%d", strings.Join(InstalledProtocols(config), "+"), FormatPorts(ListeningPorts(config)), RouteRuleCount(config))
}
