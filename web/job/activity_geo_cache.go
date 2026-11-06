package job

import (
	"container/list"
	"sync"
	"time"
)

const activityGeoCacheCapacity = 2048

type activityGeoItem struct {
	ip    string
	entry activityGeoCacheEntry
}

// activityGeoLRU retains only a bounded set of recent, unexpired lookups.
type activityGeoLRU struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*list.Element
	order    *list.List
}

func newActivityGeoCache(capacity int) *activityGeoLRU {
	if capacity < 1 {
		capacity = 1
	}
	return &activityGeoLRU{capacity: capacity, items: make(map[string]*list.Element), order: list.New()}
}

var activityGeoCache = newActivityGeoCache(activityGeoCacheCapacity)

func (c *activityGeoLRU) remove(e *list.Element) {
	delete(c.items, e.Value.(activityGeoItem).ip)
	c.order.Remove(e)
}

func (c *activityGeoLRU) get(ip string, now time.Time) (activityGeoCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.items[ip]
	if e == nil {
		return activityGeoCacheEntry{}, false
	}
	item := e.Value.(activityGeoItem)
	if !now.Before(item.entry.Expires) {
		c.remove(e)
		return activityGeoCacheEntry{}, false
	}
	c.order.MoveToFront(e)
	return item.entry, true
}

func (c *activityGeoLRU) pruneLocked(now time.Time) {
	for e := c.order.Back(); e != nil; {
		previous := e.Prev()
		if !now.Before(e.Value.(activityGeoItem).entry.Expires) {
			c.remove(e)
		}
		e = previous
	}
}

func (c *activityGeoLRU) prune(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked(now)
}

func (c *activityGeoLRU) put(ip string, entry activityGeoCacheEntry, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !now.Before(entry.Expires) {
		if e := c.items[ip]; e != nil {
			c.remove(e)
		}
		return
	}
	if e := c.items[ip]; e != nil {
		e.Value = activityGeoItem{ip: ip, entry: entry}
		c.order.MoveToFront(e)
		return
	}
	if len(c.items) >= c.capacity {
		c.pruneLocked(now)
	}
	for len(c.items) >= c.capacity {
		c.remove(c.order.Back())
	}
	c.items[ip] = c.order.PushFront(activityGeoItem{ip: ip, entry: entry})
}
