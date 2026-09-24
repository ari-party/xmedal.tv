package main

import (
	"sync"
	"time"
)

const memoryCacheCap = 10000

type memoryEntry struct {
	contentURL string
	expiresAt  time.Time
}

var (
	memoryMu    sync.RWMutex
	memoryStore = map[string]memoryEntry{}
)

func memoryGet(key string) string {
	memoryMu.RLock()
	entry, ok := memoryStore[key]
	memoryMu.RUnlock()

	if !ok || time.Now().After(entry.expiresAt) {
		return ""
	}

	return entry.contentURL
}

func memorySet(key, contentURL string, ttl time.Duration) {
	now := time.Now()

	memoryMu.Lock()
	defer memoryMu.Unlock()

	if len(memoryStore) >= memoryCacheCap {
		for k, entry := range memoryStore {
			if now.After(entry.expiresAt) {
				delete(memoryStore, k)
			}
		}
		// still full of live entries, so throw the lot out rather than grow forever
		if len(memoryStore) >= memoryCacheCap {
			clear(memoryStore)
		}
	}

	memoryStore[key] = memoryEntry{contentURL: contentURL, expiresAt: now.Add(ttl)}
}
