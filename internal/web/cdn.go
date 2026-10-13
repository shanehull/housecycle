package web

import (
	"html/template"
	"strings"
)

// CDNScript is a pinned third-party script. Integrity is the SRI sha384 hash of
// the exact file, used both as the script's integrity attribute and as a CSP
// script-src hash. Bump the version and the hash together; run
// `go run ./cmd/sri` to print the current hashes and flag drift.
type CDNScript struct {
	Name      string
	URL       string
	Integrity string
}

// IntegrityAttr renders the integrity attribute, kept raw so html/template does
// not escape the "+" and "/" in the base64 hash.
func (s CDNScript) IntegrityAttr() template.HTMLAttr {
	return template.HTMLAttr(`integrity="` + s.Integrity + `"`)
}

// CDNScripts is the single source of truth for third-party frontend scripts.
var CDNScripts = []CDNScript{
	{
		Name:      "htmx",
		URL:       "https://cdn.jsdelivr.net/npm/htmx.org@2.0.10/dist/htmx.min.js",
		Integrity: "sha384-H5SrcfygHmAuTDZphMHqBJLc3FhssKjG7w/CeCpFReSfwBWDTKpkzPP8c+cLsK+V",
	},
	{
		Name:      "chart.js",
		URL:       "https://cdn.jsdelivr.net/npm/chart.js@4.5.1/dist/chart.umd.min.js",
		Integrity: "sha384-jb8JQMbMoBUzgWatfe6COACi2ljcDdZQ2OxczGA3bGNeWe+6DChMTBJemed7ZnvJ",
	},
}

// cspScriptHashes returns the CDN hashes quoted for a CSP script-src directive.
func cspScriptHashes() string {
	hashes := make([]string, len(CDNScripts))
	for i, s := range CDNScripts {
		hashes[i] = "'" + s.Integrity + "'"
	}
	return strings.Join(hashes, " ")
}
