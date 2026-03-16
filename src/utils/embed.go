package utils

import (
	"encoding/json"
	"regexp"
)

var (
	jsonldScriptRegex  = regexp.MustCompile(`<script[^>]*type="application/ld\+json"[^>]*>([\s\S]*?)</script>`)
	hydrationDataRegex = regexp.MustCompile(`<script>\s*var hydrationData=([\s\S]*?)</script>`)
)

func ExtractHydrationData(html string) string {
	match := hydrationDataRegex.FindStringSubmatch(html)
	if len(match) < 2 {
		return ""
	}
	data := match[1]
	if len(data) > 0 && data[len(data)-1] == ';' {
		data = data[:len(data)-1]
	}
	return data
}

func ExtractJSONLDScripts(html string) []string {
	matches := jsonldScriptRegex.FindAllStringSubmatch(html, -1)
	if len(matches) == 0 {
		return nil
	}

	result := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 && match[1] != "" {
			result = append(result, match[1])
		}
	}

	return result
}

func ParseJSONLD(data string) (map[string]any, error) {
	var result map[string]any
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil, err
	}

	return result, nil
}
