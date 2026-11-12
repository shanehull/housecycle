package source

import (
	"context"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"

	"github.com/shanehull/housecycle/internal/model"
)

// OECD fetches the OECD nominal house price index and the house price to rent
// ratio for Australia (quarterly, 2015 = 100) from the keyless OECD SDMX API.
// The house price index is the price leg of price-to-income and price-to-rent.
type OECD struct{}

func (OECD) Name() string { return "OECD" }

const oecdBase = "https://sdmx.oecd.org/public/rest/data/" +
	"OECD.ECO.MPD,DSD_AN_HOUSE_PRICES@DF_HOUSE_PRICES,1.0/AUS.Q."

func (OECD) Fetch(ctx context.Context, cfg Config) ([]model.Series, error) {
	return fetchAll(ctx,
		func(ctx context.Context) ([]model.Series, error) {
			return oecdSeries(ctx, cfg, "HPI", "oecd.au.hpi", "Nominal house price index, Australia (2015 = 100)")
		},
		func(ctx context.Context) ([]model.Series, error) {
			return oecdSeries(ctx, cfg, "HPI_RPI", "oecd.au.price_to_rent", "House price to rent ratio, Australia (2015 = 100)")
		},
	)
}

func oecdSeries(ctx context.Context, cfg Config, measure, id, label string) ([]model.Series, error) {
	body, err := get(ctx, cfg, oecdBase+measure+".?format=csvfile", "text/csv", nil)
	if err != nil {
		return nil, err
	}
	points, err := parsePeriodValueCSV(body)
	if err != nil {
		return nil, fmt.Errorf("oecd %s: %w", measure, err)
	}
	if len(points) == 0 {
		return nil, fmt.Errorf("oecd %s: no points", measure)
	}
	return []model.Series{{
		ID: id, Label: label, Unit: "index", Freq: "Q", Source: "OECD", Points: points,
	}}, nil
}

// parsePeriodValueCSV reads a CSV with TIME_PERIOD and OBS_VALUE columns into points.
func parsePeriodValueCSV(body []byte) ([]model.Point, error) {
	r := csv.NewReader(strings.NewReader(string(body)))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("no rows")
	}
	header := rows[0]
	periodCol, valueCol := -1, -1
	for i, h := range header {
		switch strings.TrimSpace(h) {
		case "TIME_PERIOD":
			periodCol = i
		case "OBS_VALUE":
			valueCol = i
		}
	}
	if periodCol < 0 || valueCol < 0 {
		return nil, fmt.Errorf("unexpected header: %v", header)
	}
	var points []model.Point
	for _, row := range rows[1:] {
		if periodCol >= len(row) || valueCol >= len(row) {
			continue
		}
		t, err := parsePeriod(row[periodCol])
		if err != nil {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(row[valueCol]), 64)
		if err != nil {
			continue
		}
		points = append(points, model.Point{Date: t, Value: v})
	}
	sortPoints(points)
	return points, nil
}
