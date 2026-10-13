// Package buildinfo carries values injected at build time.
package buildinfo

// GitTag is the release tag, set at build time with -ldflags -X.
var GitTag string
