package main

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"sync"
	"time"

	"xmedaltv/src/utils"
)

const downloadLinger = time.Minute

type download struct {
	file        string
	contentType string
	mu          sync.Mutex
	cond        *sync.Cond
	size        int64
	done        bool
	err         error
}

var (
	downloadsMu sync.Mutex
	downloads   = map[string]*download{}
)

func getDownload(key string) *download {
	downloadsMu.Lock()
	defer downloadsMu.Unlock()

	return downloads[key]
}

func startDownload(key, src string) *download {
	downloadsMu.Lock()
	defer downloadsMu.Unlock()

	if d, ok := downloads[key]; ok {
		return d
	}

	d := &download{contentType: contentTypeOf(src)}
	d.cond = sync.NewCond(&d.mu)

	f, err := os.CreateTemp("", "xmedal-*")
	if err != nil {
		d.done, d.err = true, err
		return d
	}
	d.file = f.Name()
	downloads[key] = d

	go d.run(key, src, f)

	return d
}

func contentTypeOf(src string) string {
	if parsed, err := url.Parse(src); err == nil {
		if contentType := mime.TypeByExtension(path.Ext(parsed.Path)); contentType != "" {
			return contentType
		}
	}

	// .mp4 isn't in go's builtin table and distroless has no /etc/mime.types
	return "video/mp4"
}

func (d *download) run(key, src string, f *os.File) {
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	err := d.fetch(ctx, key, src, f)
	f.Close()
	timing(start, "download", "key", key, "bytes", d.size, "ok", err == nil)
	if err != nil {
		utils.Logger().Error("download failed", "key", key, "error", err)
	}

	d.mu.Lock()
	d.done, d.err = true, err
	d.cond.Broadcast()
	d.mu.Unlock()

	// new requests redirect (or retry) from here, the file only has to outlive readers that already opened it
	downloadsMu.Lock()
	delete(downloads, key)
	downloadsMu.Unlock()
	time.AfterFunc(downloadLinger, func() { _ = os.Remove(d.file) })
}

func (d *download) fetch(ctx context.Context, key, src string, f *os.File) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", genericUserAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("content returned status %d", resp.StatusCode)
	}

	// the url the redirects landed on is the presigned asset
	cacheContentURL(key, resp.Request.URL.String())

	buf := make([]byte, 64<<10)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}

			d.mu.Lock()
			d.size += int64(n)
			d.cond.Broadcast()
			d.mu.Unlock()
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func (d *download) streamTo(ctx context.Context, w http.ResponseWriter) error {
	if d.file == "" {
		return d.err
	}

	f, err := os.Open(d.file)
	if err != nil {
		return err
	}
	defer f.Close()

	stop := context.AfterFunc(ctx, func() {
		d.mu.Lock()
		d.cond.Broadcast()
		d.mu.Unlock()
	})
	defer stop()

	rc := http.NewResponseController(w)

	var sent int64
	for {
		d.mu.Lock()
		for sent == d.size && !d.done && ctx.Err() == nil {
			d.cond.Wait()
		}
		size, done, downloadErr := d.size, d.done, d.err
		d.mu.Unlock()

		if err := ctx.Err(); err != nil {
			return err
		}
		if sent == size && done {
			return downloadErr
		}

		n, err := io.CopyN(w, f, size-sent)
		sent += n
		if err != nil {
			return err
		}
		_ = rc.Flush()
	}
}
