package metrics

import (
	"sort"
	"sync"
)

type HotKeyTracker struct {
	counts    map[string]int
	threshold int
	mu        sync.RWMutex
}

func NewHotKeyTracker(threshold int) *HotKeyTracker {
	return &HotKeyTracker{
		counts:    make(map[string]int),
		threshold: threshold,
	}
}

func (h *HotKeyTracker) Record(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.counts[key]++
}

func (h *HotKeyTracker) IsHot(key string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.counts[key] >= h.threshold
}

func (h *HotKeyTracker) TopKeys(n int) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	type kv struct {
		Key   string
		Count int
	}

	var kvs []kv
	for k, v := range h.counts {
		kvs = append(kvs, kv{k, v})
	}

	sort.Slice(kvs, func(i, j int) bool {
		return kvs[i].Count > kvs[j].Count
	})

	limit := n
	if len(kvs) < n {
		limit = len(kvs)
	}

	var top []string
	for i := 0; i < limit; i++ {
		top = append(top, kvs[i].Key)
	}
	return top
}

func (h *HotKeyTracker) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.counts = make(map[string]int)
}
