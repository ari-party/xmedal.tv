package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestDownloadStreamsWhileWriting(t *testing.T) {
	t.Setenv("REDIS_URL", "redis://127.0.0.1:1")

	body := bytes.Repeat([]byte("medal"), 100_000)
	release := make(chan struct{})

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/content" {
			http.Redirect(w, r, "/asset?auth=exp=4102444800~hmac=x", http.StatusFound)
			return
		}
		w.Write(body[:1000])
		w.(http.Flusher).Flush()
		<-release
		w.Write(body[1000:])
	}))
	defer origin.Close()

	d := startDownload("clip", origin.URL+"/content")
	if again := startDownload("clip", origin.URL+"/content"); again != d {
		t.Fatal("second start did not reuse the running download")
	}

	var wg sync.WaitGroup
	recorders := []*httptest.ResponseRecorder{httptest.NewRecorder(), httptest.NewRecorder()}
	for _, rec := range recorders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := d.streamTo(context.Background(), rec); err != nil {
				t.Error(err)
			}
		}()
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		d.mu.Lock()
		size := d.size
		d.mu.Unlock()
		if size >= 1000 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first bytes never hit the disk")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := memoryGet("clip"); got != origin.URL+"/asset?auth=exp=4102444800~hmac=x" {
		t.Fatalf("presigned url not cached before the body finished, got %q", got)
	}

	close(release)
	wg.Wait()

	for _, rec := range recorders {
		if !bytes.Equal(rec.Body.Bytes(), body) {
			t.Fatalf("streamed %d bytes, want %d", rec.Body.Len(), len(body))
		}
	}
	if getDownload("clip") != nil {
		t.Fatal("finished download still registered")
	}
}

func TestContentTypeOf(t *testing.T) {
	for src, want := range map[string]string{
		"https://cdn.medal.tv/mediac/x.mp4?auth=exp=1~hmac=x":                   "video/mp4",
		"https://cdn.medal.tv/ugcc/content-thumbnail/x-0.jpg?auth=exp=1~hmac=x": "image/jpeg",
		"https://medal.tv/api/content/x/stream":                                 "video/mp4",
	} {
		if got := contentTypeOf(src); got != want {
			t.Errorf("contentTypeOf(%q) = %q, want %q", src, got, want)
		}
	}
}
