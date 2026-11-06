package job

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestActivityGeoCacheExpiryAndCapacity(t *testing.T) {
	now := time.Unix(1000, 0)
	c := newActivityGeoCache(3)
	put := func(key string, ttl time.Duration) {
		c.put(key, activityGeoCacheEntry{Country: key, Expires: now.Add(ttl)}, now)
	}
	put("a", time.Hour)
	put("b", time.Hour)
	put("c", time.Hour)
	if _, ok := c.get("a", now); !ok {
		t.Fatal("live entry missing")
	}
	put("d", time.Hour)
	if _, ok := c.get("b", now); ok {
		t.Fatal("least recently used entry retained at capacity")
	}
	if _, ok := c.get("a", now); !ok {
		t.Fatal("recent entry evicted")
	}
	c.put("a", activityGeoCacheEntry{Country: "updated", Expires: now.Add(time.Minute)}, now)
	if e, ok := c.get("a", now); !ok || e.Country != "updated" {
		t.Fatal("update not visible")
	}
	if _, ok := c.get("a", now.Add(time.Minute)); ok {
		t.Fatal("expired entry returned")
	}
	c.prune(now.Add(time.Hour))
	if len(c.items) != 0 || c.order.Len() != 0 {
		t.Fatal("expired entries retained after prune")
	}
	for i := 0; i < 10000; i++ {
		put(fmt.Sprint(i), time.Hour)
	}
	if len(c.items) != 3 || c.order.Len() != 3 {
		t.Fatal("unbounded cache")
	}
	c.prune(now.Add(2 * time.Hour))
	if len(c.items) != 0 {
		t.Fatal("expired distinct lookups remain reachable")
	}
}

func TestActivityGeoCacheExpiresBeforeLRUEviction(t *testing.T) {
	now := time.Now()
	c := newActivityGeoCache(2)
	c.put("live", activityGeoCacheEntry{Expires: now.Add(time.Hour)}, now)
	c.put("expired", activityGeoCacheEntry{Expires: now.Add(time.Second)}, now)
	c.put("new", activityGeoCacheEntry{Expires: now.Add(time.Hour)}, now.Add(2*time.Second))
	if _, ok := c.get("live", now.Add(2*time.Second)); !ok {
		t.Fatal("evicted live entry before expired entry")
	}
	if len(c.items) != 2 {
		t.Fatal("bad cache size")
	}
}

func TestActivityGeoCacheConcurrentBound(t *testing.T) {
	c := newActivityGeoCache(32)
	now := time.Now()
	var wg sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				key := fmt.Sprintf("%d-%d", worker, i)
				c.put(key, activityGeoCacheEntry{Expires: now.Add(time.Minute)}, now)
				c.get(key, now)
				if i%19 == 0 {
					c.prune(now)
				}
			}
		}(worker)
	}
	wg.Wait()
	c.mu.Lock()
	if len(c.items) > 32 || c.order.Len() != len(c.items) {
		t.Fatal("cache bound/list invariant failed")
	}
	c.mu.Unlock()
	c.prune(now.Add(time.Minute))
	if len(c.items) != 0 {
		t.Fatal("concurrent entries not reclaimed")
	}
}

type activityGeoTestTransport struct{ calls int }

func (r *activityGeoTestTransport) RoundTrip(*http.Request) (*http.Response, error) {
	r.calls++
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"success":true,"country":"test","connection":{"asn":42}}`)), Header: make(http.Header)}, nil
}

func TestActivityGeoLookupRetainsTTLAndPrivateBypass(t *testing.T) {
	oldCache, oldTransport := activityGeoCache, http.DefaultTransport
	activityGeoCache = newActivityGeoCache(8)
	transport := &activityGeoTestTransport{}
	http.DefaultTransport = transport
	t.Cleanup(func() { activityGeoCache = oldCache; http.DefaultTransport = oldTransport })
	start := time.Now()
	for i := 0; i < 2; i++ {
		if e := activityGeoForIP("198.18.0.1"); e.Country != "test" || e.ASN != 42 {
			t.Fatal("lookup changed")
		}
	}
	if transport.calls != 1 {
		t.Fatal("cache miss for live entry")
	}
	activityGeoForIP("127.0.0.1")
	if transport.calls != 1 {
		t.Fatal("private IP caused network lookup")
	}
	if e, ok := activityGeoCache.get("198.18.0.1", start.Add(23*time.Hour)); !ok || e.Country != "test" {
		t.Fatal("success TTL was shortened")
	}
	activityGeoCache.prune(start.Add(25 * time.Hour))
	if len(activityGeoCache.items) != 0 {
		t.Fatal("expired lookup retained")
	}
}
