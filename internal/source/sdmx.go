package source

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// sdmxDim is an SDMX dimension with its ordered value ids.
type sdmxDim struct {
	ID     string `json:"id"`
	Values []struct {
		ID string `json:"id"`
	} `json:"values"`
}

// sdmxSeries is one series: observations keyed by period.
type sdmxSeries struct {
	Obs map[string]float64
}

// sdmxDataset is the parsed form of an ABS SDMX-JSON data message.
type sdmxDataset struct {
	Series []sdmxSeries
}

type sdmxResponse struct {
	Data struct {
		Structures []struct {
			Dimensions struct {
				Observation []sdmxDim `json:"observation"`
			} `json:"dimensions"`
		} `json:"structures"`
		DataSets []struct {
			Series map[string]struct {
				Observations map[string][]json.RawMessage `json:"observations"`
			} `json:"series"`
		} `json:"dataSets"`
	} `json:"data"`
	Errors []struct {
		Title string `json:"title"`
	} `json:"errors"`
}

// parseSDMX decodes an ABS SDMX-JSON message into series keyed by period.
// Observation values are the first element of each observation tuple.
func parseSDMX(raw []byte) (*sdmxDataset, error) {
	var resp sdmxResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode sdmx: %w", err)
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("sdmx error: %s", resp.Errors[0].Title)
	}
	if len(resp.Data.Structures) == 0 || len(resp.Data.DataSets) == 0 {
		return nil, fmt.Errorf("sdmx: empty response")
	}
	var obsDim sdmxDim
	if od := resp.Data.Structures[0].Dimensions.Observation; len(od) > 0 {
		obsDim = od[0]
	}

	ds := &sdmxDataset{}
	for _, raw := range resp.Data.DataSets[0].Series {
		ser := sdmxSeries{Obs: map[string]float64{}}
		for obsKey, tuple := range raw.Observations {
			oi, err := strconv.Atoi(obsKey)
			if err != nil || oi >= len(obsDim.Values) || len(tuple) == 0 {
				continue
			}
			v, err := parseJSONNumber(tuple[0])
			if err != nil {
				continue
			}
			ser.Obs[obsDim.Values[oi].ID] = v
		}
		ds.Series = append(ds.Series, ser)
	}
	return ds, nil
}

func parseJSONNumber(raw json.RawMessage) (float64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, fmt.Errorf("null")
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0, err
	}
	return f, nil
}
