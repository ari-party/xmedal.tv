package utils

import (
	"strings"

	"github.com/avct/uasurfer"
)

func IsBot(userAgent string) bool {
	ua := strings.TrimSpace(userAgent)
	if ua == "" {
		return false
	}

	result := uasurfer.Parse(ua)
	return result.IsBot()
}
