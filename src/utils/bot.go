package utils

import (
	"strings"

	"github.com/mssola/useragent"
)

func IsBot(userAgent string) bool {
	ua := strings.TrimSpace(userAgent)
	if ua == "" {
		return false
	}

	result := useragent.New(ua)
	return result.Bot()
}
