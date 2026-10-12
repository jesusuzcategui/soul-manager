// Package version exposes the Soul App version string.
package version

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var raw string

// Value is the application version, embedded at build time from the VERSION
// file in this directory. To release, bump VERSION and push a matching
// vX.Y.Z tag; the release workflow enforces that the two agree.
var Value = strings.TrimSpace(raw)
