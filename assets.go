// Package nurtur exposes the assets compiled into the Nurtur binary.
package nurtur

import "embed"

// Assets holds the templates, static files, and SQL migrations. Everything the
// app serves or applies at startup lives in the binary; there are no runtime
// file paths for web assets.
//
//go:embed all:web all:migrations
var Assets embed.FS
