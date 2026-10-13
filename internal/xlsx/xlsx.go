// Package xlsx reads cell values from an Office Open XML (.xlsx) workbook using
// only the standard library. It is deliberately small: ABS data cubes are simple
// sheets of strings and numbers.
package xlsx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Read parses every worksheet, returning rows of cell text keyed by sheet name.
func Read(data []byte) (map[string][][]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		files[f.Name] = b
	}

	shared, err := parseShared(files["xl/sharedStrings.xml"])
	if err != nil {
		return nil, err
	}
	targets := sheetTargets(files["xl/workbook.xml"], files["xl/_rels/workbook.xml.rels"])

	out := make(map[string][][]string, len(targets))
	for name, target := range targets {
		rows, err := parseSheet(files["xl/"+target], shared)
		if err != nil {
			return nil, fmt.Errorf("sheet %s: %w", name, err)
		}
		out[name] = rows
	}
	return out, nil
}

func parseShared(data []byte) ([]string, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var sst struct {
		SI []struct {
			T string `xml:"t"`
			R []struct {
				T string `xml:"t"`
			} `xml:"r"`
		} `xml:"si"`
	}
	if err := xml.Unmarshal(data, &sst); err != nil {
		return nil, err
	}
	out := make([]string, len(sst.SI))
	for i, si := range sst.SI {
		var b strings.Builder
		b.WriteString(si.T)
		for _, r := range si.R {
			b.WriteString(r.T)
		}
		out[i] = b.String()
	}
	return out, nil
}

func sheetTargets(workbook, rels []byte) map[string]string {
	var wb struct {
		Sheets struct {
			Sheet []struct {
				Name string `xml:"name,attr"`
				RID  string `xml:"id,attr"`
			} `xml:"sheet"`
		} `xml:"sheets"`
	}
	if err := xml.Unmarshal(workbook, &wb); err != nil {
		return nil
	}
	var rs struct {
		Rel []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal(rels, &rs); err != nil {
		return nil
	}
	byID := make(map[string]string, len(rs.Rel))
	for _, r := range rs.Rel {
		byID[r.ID] = strings.TrimPrefix(r.Target, "/xl/")
	}
	out := make(map[string]string, len(wb.Sheets.Sheet))
	for _, s := range wb.Sheets.Sheet {
		if t, ok := byID[s.RID]; ok {
			out[s.Name] = t
		}
	}
	return out
}

func parseSheet(data []byte, shared []string) ([][]string, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty sheet")
	}
	var sh struct {
		Rows []struct {
			R string `xml:"r,attr"`
			C []struct {
				R  string `xml:"r,attr"`
				T  string `xml:"t,attr"`
				V  string `xml:"v"`
				Is struct {
					T string `xml:"t"`
				} `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(data, &sh); err != nil {
		return nil, err
	}

	out := make([][]string, 0, len(sh.Rows))
	for _, row := range sh.Rows {
		rowIdx := atoi(row.R)
		for len(out) < rowIdx {
			out = append(out, nil)
		}
		var cells []string
		for _, c := range row.C {
			col := colIndex(c.R)
			for len(cells) <= col {
				cells = append(cells, "")
			}
			cells[col] = cellValue(c.T, c.V, c.Is.T, shared)
		}
		out[rowIdx-1] = cells
	}
	return out, nil
}

func cellValue(typ, v, inline string, shared []string) string {
	switch typ {
	case "s":
		if i := atoi(v); i >= 0 && i < len(shared) {
			return shared[i]
		}
		return ""
	case "inlineStr":
		return inline
	default:
		return v
	}
}

func colIndex(ref string) int {
	n := 0
	for _, r := range ref {
		if r >= 'A' && r <= 'Z' {
			n = n*26 + int(r-'A'+1)
		} else {
			break
		}
	}
	return n - 1
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
