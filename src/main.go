package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
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

	errNotFound = errors.New("content not found")

	fetchGroup singleflight.Group

	genericUserAgent = "Mozilla/5.0 (compatible; xmedaltv/1.0; +https://xmedal.tv)"
)

func fetchContentURL(ctx context.Context, url string) (string, error) {
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

	contentURL, err := utils.ExtractContentURL(string(body))
	if err != nil {
		return "", err
	}

	return contentURL, nil
}

func redirect(w http.ResponseWriter, destination string, status int) {
	w.Header().Set("Location", destination)
	w.WriteHeader(status)
}

func handleContent(w http.ResponseWriter, r *http.Request, nodeEnv string) {
	log := utils.Logger()
	path := strings.TrimPrefix(r.URL.Path, "/")
	key := slug.Make(path)
	fullURL := utils.GetFullURL(path)

	if nodeEnv == "development" || utils.IsBot(r.UserAgent()) {
		ctx := r.Context()
		contentURL, err := redis.GetCachedContentURL(ctx, key)
		if err != nil {
			log.Error("failed to read from cache", "error", err)
		}

		if contentURL == "" {
			result, fetchErr, _ := fetchGroup.Do(key, func() (interface{}, error) {
				fetchedURL, innerErr := fetchContentURL(ctx, fullURL)
				if innerErr != nil {
					return "", innerErr
				}

				if err := redis.SetCachedContentURL(ctx, key, fetchedURL); err != nil {
					log.Error("failed to cache content url", "error", err)
				}

				return fetchedURL, nil
			})

			if fetchErr != nil {
				if errors.Is(fetchErr, errNotFound) {
					http.NotFound(w, r)
					return
				}

				log.Error("failed to fetch content url", "error", fetchErr)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}

			contentURL, _ = result.(string)
		}

		redirect(w, contentURL, http.StatusFound)
		return
	}

	redirect(w, fullURL, http.StatusFound)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := redis.Ping(ctx); err != nil {
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

	// Initialise Redis connection early
	redis.Client()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			redirect(w, "https://github.com/ari-party/xmedal.tv#xmedaltv", http.StatusFound)
			return
		}

		handleContent(w, r, cfg.NodeEnv)
	})

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	log.Info("server listening", "addr", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Error("server stopped", "error", err)
	}
}
