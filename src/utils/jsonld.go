package utils

import (
	"encoding/json"
	"regexp"
)

var scriptRegex = regexp.MustCompile(`<script type="application/ld\+json">([\s\S]*?)</script>`)

func ExtractJSONLDScripts(html string) []string {
	matches := scriptRegex.FindAllStringSubmatch(html, -1)
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
