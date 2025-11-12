package utils

import (
	"errors"
	"fmt"
)

func GetFullURL(path string) string {
	return fmt.Sprintf("https://medal.tv/%s", path)
}

func ExtractContentURL(html string) (string, error) {
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
