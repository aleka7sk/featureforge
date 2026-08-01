package http

import (
	"bytes"
	"encoding/json"
)

// optionalString preserves the distinction between an omitted JSON member and
// a member explicitly supplied as an empty string or null. Commands use that
// distinction to allow omission only on a validated replay path.
type optionalString struct {
	Value   string
	Present bool
}

func (o *optionalString) UnmarshalJSON(data []byte) error {
	o.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		o.Value = ""
		return nil
	}
	return json.Unmarshal(data, &o.Value)
}

func optionalStringPointer(value optionalString) *string {
	if !value.Present {
		return nil
	}
	v := value.Value
	return &v
}
