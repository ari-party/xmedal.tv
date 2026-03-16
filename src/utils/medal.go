package utils

import (
	"errors"
	"fmt"
	"net/url"
)

func GetFullURL(path string) string {
	fullURL := fmt.Sprintf("https://medal.tv/%s", path)

	parsedURL, err := url.Parse(fullURL)
	if err != nil {
		return fullURL
	}

	parsedURL.RawQuery = ""
	parsedURL.Fragment = ""

	return parsedURL.String()
}

func extractContentURLFromHydration(html string) string {
	hydrationData := ExtractHydrationData(html)
	if hydrationData == "" {
		return ""
	}
	doc, err := ParseJSONLD(hydrationData)
	if err != nil {
		return ""
	}
	clips, ok := doc["clips"].(map[string]any)
	if !ok || len(clips) == 0 {
		return ""
	}
	for _, clip := range clips {
		clipMap, ok := clip.(map[string]any)
		if !ok {
			continue
		}
		if url, ok := clipMap["contentUrl"].(string); ok && url != "" {
			return url
		}
	}
	return ""
}

func ExtractContentURL(html string) (string, error) {
	if url := extractContentURLFromHydration(html); url != "" {
		return url, nil
	}

	scripts := ExtractJSONLDScripts(html)
	if len(scripts) == 0 {
		return "", errors.New("no json-ld script found")
	}

	document, err := ParseJSONLD(scripts[0])
	if err != nil {
		return "", err
	}

	if document["@type"] != "VideoObject" {
		return "", errors.New("json-ld @type is not VideoObject")
	}

	value, ok := document["contentUrl"].(string)
	if !ok || value == "" {
		return "", errors.New("json-ld contentUrl is missing")
	}

	return value, nil
}
