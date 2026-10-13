// Package cache is an in-memory cache.
package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shanehull/housecycle/internal/model"
)

// Cache holds series in memory, guarded by a mutex.
type Cache struct {
	mu         sync.RWMutex
	series     map[string]model.Series
	lastIngest time.Time
}

// New returns an empty cache.
func New() *Cache {
	return &Cache{series: map[string]model.Series{}}
}

// Put replaces the given series and records the ingest time.
func (c *Cache) Put(series []model.Series) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ser := range series {
		c.series[ser.ID] = ser
	}
	if len(series) > 0 {
		c.lastIngest = time.Now().UTC()
	}
}

// Series returns a stored series by ID, implementing signals.Loader.
func (c *Cache) Series(_ context.Context, id string) (model.Series, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ser, ok := c.series[id]
	if !ok {
		return model.Series{}, fmt.Errorf("series %q not found", id)
	}
	return ser, nil
}

// LastIngest reports when any series was last written.
func (c *Cache) LastIngest(_ context.Context) (time.Time, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastIngest, !c.lastIngest.IsZero(), nil
}
