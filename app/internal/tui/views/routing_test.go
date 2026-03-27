package views

import (
	"strings"
	"testing"
)

func TestParsePresetSelectionUsesShellProxyKeys(t *testing.T) {
	presets, err := parsePresetSelection("1,g,a,r")
	if err != nil {
		t.Fatalf("parsePresetSelection() error = %v", err)
	}
	if len(presets) != 4 {
		t.Fatalf("len(presets) = %d, want 4", len(presets))
	}
	if presets[0].Name != "openai" || presets[1].Name != "discord" || presets[2].Name != "ai-intl" || presets[3].Name != "ads" {
		t.Fatalf("parsed preset names = %#v", []string{presets[0].Name, presets[1].Name, presets[2].Name, presets[3].Name})
	}
}

func TestPresetSelectionPromptMatchesShellProxyKeys(t *testing.T) {
	v := &RoutingView{}
	prompt := v.presetSelectionPrompt()
	for _, want := range []string{"1. OpenAI/ChatGPT", "g. Discord", "a. AI服务(国际)", "r. 广告屏蔽"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}
