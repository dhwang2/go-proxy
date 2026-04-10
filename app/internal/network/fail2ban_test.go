package network

import (
	"strings"
	"testing"
)

func TestSSHDJailConfigUsesOneDayBan(t *testing.T) {
	if !strings.Contains(sshdJailConfig, "bantime = 86400") {
		t.Fatalf("sshdJailConfig = %q, want bantime = 86400", sshdJailConfig)
	}
}
