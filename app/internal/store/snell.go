package store

import (
	"bufio"
	"bytes"
	"fmt"
	"sort"
	"strings"
)

type SnellConfig struct {
	Values map[string]string
}

func ParseSnellConfig(content []byte) *SnellConfig {
	cfg := &SnellConfig{Values: map[string]string{}}
	s := bufio.NewScanner(bytes.NewReader(content))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		if k != "" {
			cfg.Values[k] = v
		}
	}
	return cfg
}

func (c *SnellConfig) MarshalText() []byte {
	if c == nil {
		return nil
	}
	keys := make([]string, 0, len(c.Values))
	for k := range c.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s = %s\n", k, c.Values[k])
	}
	return []byte(b.String())
}

func (c *SnellConfig) Get(key string) string {
	if c == nil {
		return ""
	}
	return c.Values[strings.TrimSpace(key)]
}
