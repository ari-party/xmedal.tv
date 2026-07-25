package utils

import (
	"strings"

	"github.com/mssola/useragent"
)

var embedCrawlerTokens = []string{
	"discordbot",
	"twitterbot",
	"telegrambot",
	"slackbot",
	"whatsapp",
	"facebookexternalhit",
	"redditbot",
	"linkedinbot",
	"skypeuripreview",
	"embedly",
	"iframely",
	"bluesky",
	"mastodon",
	"applebot",
}

func IsBot(userAgent string) bool {
	ua := strings.TrimSpace(userAgent)
	if ua == "" {
		return false
	}

	lower := strings.ToLower(ua)
	for _, token := range embedCrawlerTokens {
		if strings.Contains(lower, token) {
			return true
		}
	}

	return useragent.New(ua).Bot()
}
