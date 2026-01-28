// Package prompts embeds template fragments for prompt rendering.
package prompts

import "embed"

//go:embed fragments/*.tmpl
var Fragments embed.FS
