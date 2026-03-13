package service

import (
	"fmt"
	"os"
	"strings"
)

// SnellPSK extracts the PSK from a snell configuration file.
func SnellPSK(confPath string) (string, error) {
	confPath = strings.TrimSpace(confPath)
	if confPath == "" {
		return "", fmt.Errorf("empty config path")
	}
	data, err := os.ReadFile(confPath)
	if err != nil {
		return "", fmt.Errorf("read snell config: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		if strings.EqualFold(key, "psk") {
			return strings.TrimSpace(parts[1]), nil
		}
	}
	return "", fmt.Errorf("psk not found in %s", confPath)
}
