package http

import (
	"bytes"
	"encoding/json"
	"time"
)

// optionalTime preserves omission separately from an explicitly supplied zero
// or null timestamp. Application commands own the state-aware defaulting rule.
type optionalTime struct {
	Value   time.Time
	Present bool
}

func (o *optionalTime) UnmarshalJSON(data []byte) error {
	o.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		o.Value = time.Time{}
		return nil
	}
	return json.Unmarshal(data, &o.Value)
}
