// Package ingest orchestrates fetchers and stores their output.
package ingest

import (
	"context"
	"sync"

	"github.com/shanehull/housecycle/internal/cache"
	"github.com/shanehull/housecycle/internal/source"
)

// Result reports what one fetcher produced.
type Result struct {
	Source string
	Series int
	Err    error
}

// Run fetches every source concurrently and stores their results. A failing
// source never blocks the others.
func Run(ctx context.Context, c *cache.Cache, cfg source.Config, fetchers []source.Fetcher) []Result {
	results := make([]Result, len(fetchers))
	var wg sync.WaitGroup
	for i, f := range fetchers {
		wg.Go(func() {
			results[i] = Result{Source: f.Name()}
			series, err := f.Fetch(ctx, cfg)
			if err != nil {
				results[i].Err = err
				return
			}
			c.Put(series)
			results[i].Series = len(series)
		})
	}
	wg.Wait()
	return results
}
