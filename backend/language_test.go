package main

import "testing"

func TestNormalizeLanguage(t *testing.T) {
	tests := map[string]string{"": "en", "en-US": "en", "ZH-cn": "zh", "cmn-CN": "zh"}
	for input, want := range tests {
		got, ok := normalizeLanguage(input)
		if !ok || got != want {
			t.Fatalf("normalizeLanguage(%q) = %q, %v; want %q, true", input, got, ok, want)
		}
	}
	if got, ok := normalizeLanguage("fr"); ok || got != "" {
		t.Fatalf("unsupported language = %q, %v", got, ok)
	}
}
