package source

import (
	"context"
	"fmt"
	"slices"

	"github.com/shanehull/housecycle/internal/model"
)

// ABS fetches Australian Bureau of Statistics series via the SDMX REST API:
// average weekly earnings, house construction prices, dwelling transfers, and
// estimated resident population.
type ABS struct{}

func (ABS) Name() string { return "ABS" }

const absBase = "https://data.api.abs.gov.au/rest/data/"

func (a ABS) Fetch(ctx context.Context, cfg Config) ([]model.Series, error) {
	sdmx := func(flow, version, key, id, label, unit, freq string) func(context.Context) ([]model.Series, error) {
		return func(ctx context.Context) ([]model.Series, error) {
			points, err := a.fetch(ctx, cfg, flow, version, key)
			if err != nil {
				return nil, fmt.Errorf("abs %s: %w", id, err)
			}
			return []model.Series{
				{ID: id, Label: label, Unit: unit, Freq: freq, Source: "ABS", Points: points},
			}, nil
		}
	}

	return fetchAll(ctx,
		sdmx("AWE", "1.0.0", "1.1.3.7.TOT.10.AUS.S",
			"abs.au.awe", "Average weekly earnings, all employees (persons)", "$", "S"),
		sdmx("PPI", "1.1.3", "1.1451370.OUTPUT.Q",
			"abs.au.house_construction", "House construction cost (PPI output)", "index", "Q"),
		sdmx("ERP_Q", "1.0.0", "1.3.TOT.AUS.Q",
			"abs.au.population", "Estimated resident population, Australia", "persons", "Q"),
		func(ctx context.Context) ([]model.Series, error) {
			points, err := a.fetchTransfers(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("abs dwelling transfers: %w", err)
			}
			return []model.Series{
				{ID: "abs.au.dwelling_transfers", Label: "Dwelling transfers, Australia (established and attached)", Unit: "count", Freq: "Q", Source: "ABS", Points: points},
			}, nil
		},
		func(ctx context.Context) ([]model.Series, error) {
			household, err := a.fetchHouseholdBalanceSheet(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("abs household balance sheet: %w", err)
			}
			return household, nil
		},
	)
}

func (a ABS) fetch(ctx context.Context, cfg Config, flow, version, key string) ([]model.Point, error) {
	url := absBase + "ABS," + flow + "," + version + "/" + key + "?format=jsondata"
	body, err := get(ctx, cfg, url, "application/vnd.sdmx.data+json", nil)
	if err != nil {
		return nil, err
	}
	ds, err := parseSDMX(body)
	if err != nil {
		return nil, err
	}
	if len(ds.Series) == 0 {
		return nil, fmt.Errorf("no series")
	}
	points := toPoints(&ds.Series[0])
	if len(points) == 0 {
		return nil, fmt.Errorf("no points")
	}
	return points, nil
}

// fetchTransfers sums dwelling transfer counts across measures (established and
// attached) and regions (capital cities and rest of state), which RES_DWELL
// publishes separately rather than as a national total.
func (a ABS) fetchTransfers(ctx context.Context, cfg Config) ([]model.Point, error) {
	url := absBase + "ABS,RES_DWELL,1.0.0/1+2..Q?format=jsondata"
	body, err := get(ctx, cfg, url, "application/vnd.sdmx.data+json", nil)
	if err != nil {
		return nil, err
	}
	ds, err := parseSDMX(body)
	if err != nil {
		return nil, err
	}
	if len(ds.Series) == 0 {
		return nil, fmt.Errorf("no series")
	}
	sums := map[string]float64{}
	for _, s := range ds.Series {
		for period, v := range s.Obs {
			sums[period] += v
		}
	}
	points := make([]model.Point, 0, len(sums))
	for period, v := range sums {
		t, err := parsePeriod(period)
		if err != nil {
			continue
		}
		points = append(points, model.Point{Date: t, Value: v})
	}
	if len(points) == 0 {
		return nil, fmt.Errorf("no points")
	}
	sortPoints(points)
	return points, nil
}

func toPoints(ser *sdmxSeries) []model.Point {
	points := make([]model.Point, 0, len(ser.Obs))
	for period, v := range ser.Obs {
		t, err := parsePeriod(period)
		if err != nil {
			continue
		}
		points = append(points, model.Point{Date: t, Value: v})
	}
	sortPoints(points)
	return points
}

func sortPoints(points []model.Point) {
	slices.SortFunc(points, func(a, b model.Point) int { return a.Date.Compare(b.Date) })
}
