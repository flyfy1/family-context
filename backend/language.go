package main

import "strings"

func normalizeLanguage(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "en", "en-us", "en-gb":
		return "en", true
	case "zh", "zh-cn", "zh-hans", "cmn-cn":
		return "zh", true
	default:
		return "", false
	}
}
