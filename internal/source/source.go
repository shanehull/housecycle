// Package source fetches raw data from external providers and normalises it into model.Series.
package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shanehull/housecycle/internal/model"
)

// Config controls how sources fetch.
type Config struct {
	Client *http.Client
}

// Fetcher retrieves a set of series from one provider.
type Fetcher interface {
	Name() string
	Fetch(ctx context.Context, cfg Config) ([]model.Series, error)
}

// All returns the default set of fetchers. Every source is public and keyless.
func All() []Fetcher {
	return []Fetcher{OECD{}, ABS{}, RBA{}}
}

// fetchAll runs each job concurrently and concatenates the series they return.
// It waits for every job regardless of failures and reports the first error.
func fetchAll(ctx context.Context, jobs ...func(context.Context) ([]model.Series, error)) ([]model.Series, error) {
	var wg sync.WaitGroup
	results := make([][]model.Series, len(jobs))
	errs := make([]error, len(jobs))
	for i, job := range jobs {
		wg.Go(func() {
			results[i], errs[i] = job(ctx)
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	var out []model.Series
	for _, r := range results {
		out = append(out, r...)
	}
	return out, nil
}

func (c Config) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func get(ctx context.Context, c Config, url, accept string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d: %s", url, resp.StatusCode, truncate(body, 200))
	}
	return body, nil
}

func truncate(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) > n {
		return s[:n]
	}
	return s
}

// parsePeriod converts the various period strings used by the sources into a period-start date.
//
//	"2026-Q2"   -> 2026-04-01
//	"2026-06"   -> 2026-06-01
//	"2026-06-01"-> 2026-06-01
//	"1994-S2"   -> 1994-11-01 (half-yearly)
//	"11/10/2026"-> 2026-10-11
func parsePeriod(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty period")
	}
	if strings.ContainsAny(s, "Qq") {
		y, q, err := parseNumbered(s, "Q")
		if err != nil {
			return time.Time{}, err
		}
		return time.Date(y, time.Month((q-1)*3+1), 1, 0, 0, 0, 0, time.UTC), nil
	}
	if strings.ContainsAny(s, "Ss") {
		y, half, err := parseNumbered(s, "S")
		if err != nil {
			return time.Time{}, err
		}
		month := 5 // S1 ends May
		if half == 2 {
			month = 11 // S2 ends November
		}
		return time.Date(y, time.Month(month), 1, 0, 0, 0, 0, time.UTC), nil
	}
	for _, layout := range []string{"2006-01-02", "02-Jan-2006", "2006-01", "02/01/2006", "2/1/2006", "2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised period %q", s)
}

// parseNumbered parses strings like "2026-Q2" or "1994-S2" into year and index.
func parseNumbered(s, marker string) (year, index int, err error) {
	i := strings.IndexAny(s, strings.ToUpper(marker)+strings.ToLower(marker))
	if i < 0 {
		return 0, 0, fmt.Errorf("not %s: %q", marker, s)
	}
	year, err = strconv.Atoi(strings.TrimRight(strings.TrimSpace(s[:i]), "-_ "))
	if err != nil {
		return 0, 0, err
	}
	index, err = strconv.Atoi(strings.TrimSpace(s[i+1:]))
	if err != nil {
		return 0, 0, err
	}
	return year, index, nil
}
