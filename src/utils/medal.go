package utils

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func ExtractMedalExpiry(contentURL string) time.Duration {
	const fallback = 12 * time.Hour

	parsed, err := url.Parse(contentURL)
	if err != nil {
		return fallback
	}

	// auth value looks like: exp=1773790200~data=...~hmac=...
	auth := parsed.Query().Get("auth")
	if auth == "" {
		return fallback
	}

	for _, part := range strings.Split(auth, "~") {
		if strings.HasPrefix(part, "exp=") {
			ts, err := strconv.ParseInt(strings.TrimPrefix(part, "exp="), 10, 64)
			if err != nil {
				break
			}
			if d := time.Until(time.Unix(ts, 0)); d > 0 {
				return d
			}
			break
		}
	}

	return fallback
}

var contentAPIPathBlacklist = map[string]struct{}{
	"robots.txt": {},
}

func IsContentAPIBlacklisted(segment string) bool {
	_, ok := contentAPIPathBlacklist[strings.ToLower(segment)]
	return ok
}

func ExtractClipID(path string) string {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for i := len(segments) - 1; i >= 0; i-- {
		if segments[i] != "" {
			return segments[i]
		}
	}
	return ""
}

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
