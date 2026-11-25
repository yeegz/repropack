package format

import _ "embed"

// ManifestSchema is the JSON Schema (draft 2020-12) for manifest.json.
//
//go:embed schema/manifest.schema.json
var ManifestSchema string

// EventSchema is the JSON Schema (draft 2020-12) for one events.jsonl line.
//
//go:embed schema/event.schema.json
var EventSchema string
