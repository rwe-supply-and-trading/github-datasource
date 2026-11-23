package github

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/grafana/github-datasource/pkg/dfutil"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

type actionCacheEntry struct {
	value   dfutil.Framer
	expires time.Time
}

type actionQueryCache struct {
	once    sync.Once
	gate    chan struct{}
	entries map[string]actionCacheEntry
}

// Instance-local request caching coalesces catalogue/table/summary reads. There
// are no timers, goroutines or background scans. The lock also bounds concurrent
// new Action-query work against the same installation.
func (d *Datasource) cachedActionQuery(ctx context.Context, key any, fetch func() (dfutil.Framer, error)) (dfutil.Framer, error) {
	raw, err := json.Marshal(key)
	if err != nil {
		return nil, err
	}
	cache := d.actionCache
	if cache == nil {
		return fetch()
	}
	cache.once.Do(func() { cache.gate = make(chan struct{}, 1) })
	select {
	case cache.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-cache.gate }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	k := string(raw)
	if e, ok := cache.entries[k]; ok && time.Now().Before(e.expires) {
		return e.value, nil
	}
	value, err := fetch()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cache.entries == nil {
		cache.entries = make(map[string]actionCacheEntry)
	}
	for key, e := range cache.entries {
		if time.Now().After(e.expires) {
			delete(cache.entries, key)
		}
	}
	if len(cache.entries) >= 32 {
		clear(cache.entries)
	}
	cache.entries[k] = actionCacheEntry{value: value, expires: time.Now().Add(30 * time.Second)}
	return value, nil
}

type actionFrames data.Frames

func (r actionFrames) Frames() data.Frames { return data.Frames(r) }
