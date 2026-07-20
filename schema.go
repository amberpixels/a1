package a1

import "sort"

// JSON-schema builders for structured outputs. Small on purpose: schemas stay
// plain map[string]any (pass hand-written ones freely); these only remove the
// literal-key boilerplate. Descriptions matter — the model reads them like
// prompt text, so write them for the model.

// Obj builds an object schema from its properties. required lists the
// mandatory property names; when omitted, every property is required (the
// common case — and structured outputs reject optional-by-absence anyway).
// additionalProperties is always false, as structured outputs require.
func Obj(props map[string]any, required ...string) map[string]any {
	if len(required) == 0 {
		required = make([]string, 0, len(props))
		for name := range props {
			required = append(required, name)
		}
		sort.Strings(required)
	}
	return map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             required,
		"additionalProperties": false,
	}
}

// Prop builds a string property with a description.
func Prop(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

// Enum builds a string property constrained to the given values.
func Enum(description string, values ...string) map[string]any {
	p := Prop(description)
	p["enum"] = values
	return p
}
