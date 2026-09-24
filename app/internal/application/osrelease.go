package application

import (
	"bufio"
	"os"
	"runtime"
	"strings"
	"sync"
)

// osVersion reports the distribution name from /etc/os-release, which is what a
// reader wants to see rather than the Go build target. It is read once: the file
// cannot change under a running process in any way that matters, and status is
// on the latency-sensitive local-query path.
var osVersion = sync.OnceValue(func() string {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return runtime.GOOS
	}
	defer file.Close()
	fields := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), "=")
		if !found {
			continue
		}
		fields[key] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	for _, key := range []string{"PRETTY_NAME", "NAME"} {
		if value := fields[key]; value != "" {
			return value
		}
	}
	return runtime.GOOS
})
