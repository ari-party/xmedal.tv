package utils

import "testing"

func TestIsBot(t *testing.T) {
	bots := []string{
		"Mozilla/5.0 (compatible; Discordbot/2.0; +https://discordapp.com)",
		"Mozilla/5.0 (compatible; Discordbot/2.0; +https://discordapp.com) Chrome/128.0.0.0 Safari/537.36",
		"Discordbot/2.0; +https://discordapp.com",
		"Mozilla/5.0 (compatible; Twitterbot/1.0)",
		"Mozilla/5.0 (compatible; TelegramBot/1.0; +https://telegram.org)",
		"Slackbot-LinkExpanding 1.0 (+https://api.slack.com/robots)",
		"WhatsApp/2.23.20.0",
		"facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)",
	}
	for _, ua := range bots {
		if !IsBot(ua) {
			t.Errorf("expected bot: %s", ua)
		}
	}

	humans := []string{
		"",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1",
	}
	for _, ua := range humans {
		if IsBot(ua) {
			t.Errorf("expected human: %s", ua)
		}
	}
}
