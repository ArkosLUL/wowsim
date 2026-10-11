// Package trees embeds the talent tree layouts, for Go code that runs without the checkout.
package trees

import "embed"

//go:embed *.json
var FS embed.FS
