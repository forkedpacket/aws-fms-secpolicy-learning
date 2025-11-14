package templates

import "embed"

// FS contains the embedded template files so other packages can render them
// without relying on the current working directory.
//
//go:embed *.tmpl
var FS embed.FS
