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

	// no timeout, it carries the whole video body; the request's ctx bounds it
	noRedirectClient = &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	errNotFound = errors.New("content not found")

	fetchGroup singleflight.Group

	genericUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

	contentURLFields = []string{
		"contentUrl1080p",
		"contentUrl720p",
		"contentUrl480p",
		"contentUrl360p",
		"contentUrl240p",
		"contentUrl144p",
	}

	thumbnailFields = []string{
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

func firstURLField(payload map[string]any, fields []string) string {
	for _, field := range fields {
		if value, ok := payload[field].(string); ok && value != "" {
			return value
		}
	}

	return ""
}

func stripRenditionParam(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	parts := strings.Split(parsed.RawQuery, "&")
	filtered := parts[:0]
	for _, p := range parts {
		if p != "" && !strings.HasPrefix(p, "t=") {
			filtered = append(filtered, p)
		}
	}
	parsed.RawQuery = strings.Join(filtered, "&")

	return parsed.String()
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

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest {
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

	if contentURL := firstURLField(payload, contentURLFields); contentURL != "" {
		return stripRenditionParam(contentURL), nil
	}

	// screenshots have no video to fall back from
	if thumbnail := firstURLField(payload, thumbnailFields); thumbnail != "" {
		return thumbnail, nil
	}

	return "", errors.New("no content or thumbnail url in api response")
}

func fetchContentURL(ctx context.Context, path string) (string, error) {
	clipID := utils.ExtractClipID(path)
	if clipID == "" || utils.IsContentAPIBlacklisted(clipID) {
		return "", errNotFound
	}

	return fetchViaAPI(ctx, clipID)
}

func cachedContentURL(ctx context.Context, key string) string {
	memoryStart := time.Now()
	contentURL := memoryGet(key)
	timing(memoryStart, "memory_read", "key", key, "hit", contentURL != "")

	if contentURL != "" || !isResolver() {
		return contentURL
	}

	readStart := time.Now()
	cached, err := redis.GetCachedContentURL(ctx, key)
	if err != nil {
		utils.Logger().Error("failed to read from cache", "error", err)
	}
	timing(readStart, "cache_read", "key", key, "hit", cached != "")

	if cached != "" {
		memorySet(key, cached, utils.ExtractMedalExpiry(cached))
	}

	return cached
}

func cacheContentURL(key, contentURL string) {
	ttl := utils.ExtractMedalExpiry(contentURL)
	memorySet(key, contentURL, ttl)

	// off the download's path, a slow redis would otherwise hold up the first bytes
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		writeStart := time.Now()
		if err := redis.SetCachedContentURL(ctx, key, contentURL, ttl); err != nil {
			utils.Logger().Error("failed to cache content url", "error", err)
		}
		timing(writeStart, "cache_write", "key", key)
	}()
}

func downloadFor(ctx context.Context, key, path string) (*download, error) {
	if d := getDownload(key); d != nil {
		return d, nil
	}

	fetchStart := time.Now()
	result, err, shared := fetchGroup.Do(key, func() (interface{}, error) {
		// not the caller's ctx, one crawler hanging up would kill the fetch
		// everyone else is waiting on
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()

		contentURL, err := fetchContentURL(ctx, path)
		if err != nil {
			return nil, err
		}

		return startDownload(key, contentURL), nil
	})
	timing(fetchStart, "fetch", "key", key, "deduplicated", shared)

	if err != nil {
		return nil, err
	}

	return result.(*download), nil
}

func redirect(w http.ResponseWriter, destination string, status int) {
	w.Header().Set("Location", destination)
	w.WriteHeader(status)
}

func sendStreamHeaders(w http.ResponseWriter, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_ = http.NewResponseController(w).Flush()
}

func serveContent(w http.ResponseWriter, r *http.Request, path string) {
	key := slug.Make(path)

	if contentURL := cachedContentURL(r.Context(), key); contentURL != "" {
		redirect(w, contentURL, http.StatusFound)
		return
	}

	d, err := downloadFor(r.Context(), key, path)
	if err != nil {
		if errors.Is(err, errNotFound) {
			http.NotFound(w, r)
			return
		}

		utils.Logger().Error("failed to fetch content url", "path", path, "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	// before the cdn hop, so crawlers don't give up on a slow first fetch
	sendStreamHeaders(w, d.contentType)

	if err := d.streamTo(r.Context(), w); err != nil {
		if r.Context().Err() == nil {
			utils.Logger().Error("failed to stream content", "path", path, "error", err)
		}
		// the 200 is already out, so reset the connection rather than end a truncated body cleanly
		panic(http.ErrAbortHandler)
	}
}

func proxyContent(w http.ResponseWriter, r *http.Request, resolverURL, path string) {
	key := slug.Make(path)

	if contentURL := cachedContentURL(r.Context(), key); contentURL != "" {
		redirect(w, contentURL, http.StatusFound)
		return
	}

	endpoint := strings.TrimSuffix(resolverURL, "/") + "/stream?path=" + url.QueryEscape(path)

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	resp, err := noRedirectClient.Do(req)
	if err != nil {
		utils.Logger().Error("resolver request failed", "error", err)
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusFound:
		contentURL := resp.Header.Get("Location")
		memorySet(key, contentURL, utils.ExtractMedalExpiry(contentURL))
		redirect(w, contentURL, http.StatusFound)
		return
	case http.StatusOK:
		// continue
	case http.StatusNotFound:
		http.NotFound(w, r)
		return
	default:
		utils.Logger().Error("resolver returned unexpected status", "status", resp.StatusCode)
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}

	sendStreamHeaders(w, resp.Header.Get("Content-Type"))

	if _, err := io.Copy(w, resp.Body); err != nil {
		if r.Context().Err() == nil {
			utils.Logger().Error("failed to proxy stream", "path", path, "error", err)
		}
		panic(http.ErrAbortHandler)
	}
}

func handleContent(w http.ResponseWriter, r *http.Request, cfg utils.Config) {
	defer timing(time.Now(), "request", "path", r.URL.Path)

	path := strings.TrimPrefix(r.URL.Path, "/")

	if cfg.NodeEnv != "development" && !utils.IsBot(r.UserAgent()) {
		redirect(w, utils.GetFullURL(path), http.StatusFound)
		return
	}

	if cfg.ResolverURL != "" {
		proxyContent(w, r, cfg.ResolverURL, path)
		return
	}

	serveContent(w, r, path)
}

func handleStream(w http.ResponseWriter, r *http.Request) {
	defer timing(time.Now(), "stream_request", "path", r.URL.Query().Get("path"))

	path := strings.TrimPrefix(r.URL.Query().Get("path"), "/")
	if path == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}

	serveContent(w, r, path)
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
		mux.HandleFunc("/stream", handleStream)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			redirect(w, "https://github.com/ari-party/xmedal.tv#xmedaltv", http.StatusFound)
			return
		}

		handleContent(w, r, cfg)
	})

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Info("server listening", "addr", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Error("server stopped", "error", err)
	}
}
