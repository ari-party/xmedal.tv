package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gosimple/slug"
	"golang.org/x/sync/singleflight"

	"xmedaltv/src/redis"
	"xmedaltv/src/utils"
)

var (
	httpClient = &http.Client{
		Timeout: 15 * time.Second,
	}

	noRedirectClient = &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	errNotFound = errors.New("content not found")

	fetchGroup singleflight.Group

	genericUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

	apiURLFields = []string{
		"contentUrl1080p",
		"contentUrl720p",
		"contentUrl480p",
		"contentUrl360p",
		"contentUrl240p",
		"contentUrl144p",
		"thumbnail1080p",
		"thumbnail720p",
		"thumbnail480p",
		"thumbnail360p",
		"thumbnail240p",
		"thumbnail144p",
	}
)

func isResolver() bool {
	return utils.LoadConfig().ResolverURL == ""
}

func timing(start time.Time, stage string, args ...any) {
	args = append(args, "stage", stage, "ms", float64(time.Since(start).Microseconds())/1000)
	utils.Logger().Info("timing", args...)
}

func fetchViaAPI(ctx context.Context, clipID string) (string, error) {
	start := time.Now()

	apiURL := fmt.Sprintf("https://medal.tv/api/content/%s", clipID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", genericUserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", errNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("api returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	timing(start, "medal_api", "clip_id", clipID)

	var raw string
	var isThumbnail bool
	for _, field := range apiURLFields {
		if value, ok := payload[field].(string); ok && value != "" {
			raw = value
			isThumbnail = strings.HasPrefix(field, "thumbnail")
			break
		}
	}
	if raw == "" {
		return "", errors.New("no content or thumbnail url in api response")
	}

	if parsed, err := url.Parse(raw); err == nil {
		parts := strings.Split(parsed.RawQuery, "&")
		filtered := parts[:0]
		for _, p := range parts {
			if p != "" && !strings.HasPrefix(p, "t=") {
				filtered = append(filtered, p)
			}
		}
		parsed.RawQuery = strings.Join(filtered, "&")
		raw = parsed.String()
	}

	// thumbnails are already the final asset, only videos go through the cdn redirect
	if isThumbnail {
		return raw, nil
	}

	return resolvePresignedURL(ctx, raw), nil
}

func fetchViaPage(ctx context.Context, url string) (string, error) {
	defer timing(time.Now(), "page_scrape", "url", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", genericUserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// continue
	case http.StatusNotFound:
		return "", errNotFound
	default:
		return "", fmt.Errorf("unexpected status code %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return utils.ExtractContentURL(string(body))
}

// contentUrl just 302s to the real presigned asset, so we follow it ourselves
func resolvePresignedURL(ctx context.Context, contentURL string) string {
	defer timing(time.Now(), "cdn_presign")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, contentURL, nil)
	if err != nil {
		return contentURL
	}
	req.Header.Set("User-Agent", genericUserAgent)

	resp, err := noRedirectClient.Do(req)
	if err != nil {
		utils.Logger().Warn("presign resolve failed, using content url", "error", err)
		return contentURL
	}
	defer resp.Body.Close()

	location := resp.Header.Get("Location")
	if location == "" {
		return contentURL
	}

	absolute, err := resp.Request.URL.Parse(location)
	if err != nil {
		return contentURL
	}

	return absolute.String()
}

func fetchViaResolver(ctx context.Context, resolverURL, path string) (string, error) {
	defer timing(time.Now(), "resolver_hop", "path", path)

	endpoint := strings.TrimSuffix(resolverURL, "/") + "/resolve?path=" + url.QueryEscape(path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", errNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resolver returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if err != nil {
		return "", err
	}

	resolved := strings.TrimSpace(string(body))
	if resolved == "" {
		return "", errors.New("resolver returned an empty url")
	}

	return resolved, nil
}

func fetchContentURL(ctx context.Context, path string) (string, error) {
	log := utils.Logger()

	if resolver := utils.LoadConfig().ResolverURL; resolver != "" {
		return fetchViaResolver(ctx, resolver, path)
	}

	if clipID := utils.ExtractClipID(path); clipID != "" && !utils.IsContentAPIBlacklisted(clipID) {
		contentURL, err := fetchViaAPI(ctx, clipID)
		if err == nil {
			return contentURL, nil
		}
		if errors.Is(err, errNotFound) {
			return "", errNotFound
		}
		log.Warn("api fetch failed, falling back to page scrape", "clip_id", clipID, "error", err)
	}

	contentURL, err := fetchViaPage(ctx, utils.GetFullURL(path))
	if err != nil {
		return "", err
	}

	return resolvePresignedURL(ctx, contentURL), nil
}

func resolveContentURL(ctx context.Context, path string) (string, error) {
	log := utils.Logger()
	key := slug.Make(path)

	memoryStart := time.Now()
	contentURL := memoryGet(key)
	timing(memoryStart, "memory_read", "key", key, "hit", contentURL != "")

	if contentURL != "" {
		return contentURL, nil
	}

	if isResolver() {
		readStart := time.Now()
		contentURL, err := redis.GetCachedContentURL(ctx, key)
		if err != nil {
			log.Error("failed to read from cache", "error", err)
		}
		timing(readStart, "cache_read", "key", key, "hit", contentURL != "")

		if contentURL != "" {
			memorySet(key, contentURL, utils.ExtractMedalExpiry(contentURL))
			return contentURL, nil
		}
	}

	fetchStart := time.Now()
	result, err, shared := fetchGroup.Do(key, func() (interface{}, error) {
		// not the caller's ctx, one crawler hanging up would kill the fetch
		// everyone else is waiting on
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()

		fetchedURL, err := fetchContentURL(ctx, path)
		if err != nil {
			return "", err
		}

		ttl := utils.ExtractMedalExpiry(fetchedURL)
		memorySet(key, fetchedURL, ttl)

		if isResolver() {
			writeStart := time.Now()
			if err := redis.SetCachedContentURL(ctx, key, fetchedURL, ttl); err != nil {
				log.Error("failed to cache content url", "error", err)
			}
			timing(writeStart, "cache_write", "key", key)
		}

		return fetchedURL, nil
	})
	timing(fetchStart, "fetch", "key", key, "deduplicated", shared)

	if err != nil {
		return "", err
	}

	contentURL, _ = result.(string)
	return contentURL, nil
}

func redirect(w http.ResponseWriter, destination string, status int) {
	w.Header().Set("Location", destination)
	w.WriteHeader(status)
}

func handleContent(w http.ResponseWriter, r *http.Request, nodeEnv string) {
	defer timing(time.Now(), "request", "path", r.URL.Path)

	path := strings.TrimPrefix(r.URL.Path, "/")

	if nodeEnv != "development" && !utils.IsBot(r.UserAgent()) {
		redirect(w, utils.GetFullURL(path), http.StatusFound)
		return
	}

	contentURL, err := resolveContentURL(r.Context(), path)
	if err != nil {
		if errors.Is(err, errNotFound) {
			http.NotFound(w, r)
			return
		}

		utils.Logger().Error("failed to fetch content url", "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	redirect(w, contentURL, http.StatusFound)
}

func handleResolve(w http.ResponseWriter, r *http.Request) {
	defer timing(time.Now(), "resolve_request", "path", r.URL.Query().Get("path"))

	path := strings.TrimPrefix(r.URL.Query().Get("path"), "/")
	if path == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}

	contentURL, err := resolveContentURL(r.Context(), path)
	if err != nil {
		if errors.Is(err, errNotFound) {
			http.NotFound(w, r)
			return
		}

		utils.Logger().Error("failed to resolve content url", "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(contentURL))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if isResolver() && redis.Ping(ctx) != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("NOT OK"))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func main() {
	cfg := utils.LoadConfig()
	log := utils.Logger()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	if isResolver() {
		redis.Client()
		mux.HandleFunc("/resolve", handleResolve)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			redirect(w, "https://github.com/ari-party/xmedal.tv#xmedaltv", http.StatusFound)
			return
		}

		handleContent(w, r, cfg.NodeEnv)
	})

	// not 0.0.0.0, railway's private network is ipv6 only and edges couldn't reach us
	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Info("server listening", "addr", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Error("server stopped", "error", err)
	}
}
