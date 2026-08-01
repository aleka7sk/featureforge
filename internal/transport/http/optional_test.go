package http

import (
	"encoding/json"
	"testing"
	"time"
)

func TestOptionalStringPreservesPresence(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		present bool
		value   string
	}{
		{name: "omitted", body: `{}`},
		{name: "null", body: `{"value":null}`, present: true},
		{name: "empty", body: `{"value":""}`, present: true},
		{name: "value", body: `{"value":"member-1"}`, present: true, value: "member-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got struct {
				Value optionalString `json:"value"`
			}
			if err := json.Unmarshal([]byte(tt.body), &got); err != nil {
				t.Fatal(err)
			}
			if got.Value.Present != tt.present || got.Value.Value != tt.value {
				t.Fatalf("optional string = (%v, %q), want (%v, %q)", got.Value.Present, got.Value.Value, tt.present, tt.value)
			}
			ptr := optionalStringPointer(got.Value)
			if !tt.present && ptr != nil {
				t.Fatalf("omitted optional string mapped to non-nil pointer %q", *ptr)
			}
			if tt.present && (ptr == nil || *ptr != tt.value) {
				t.Fatalf("present optional string pointer = %v, want %q", ptr, tt.value)
			}
		})
	}
}

func TestOptionalTimePreservesPresence(t *testing.T) {
	want := time.Date(2026, 3, 1, 2, 3, 4, 0, time.UTC)
	tests := []struct {
		name    string
		body    string
		present bool
		value   time.Time
	}{
		{name: "omitted", body: `{}`},
		{name: "null", body: `{"value":null}`, present: true},
		{name: "value", body: `{"value":"2026-03-01T02:03:04Z"}`, present: true, value: want},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got struct {
				Value optionalTime `json:"value"`
			}
			if err := json.Unmarshal([]byte(tt.body), &got); err != nil {
				t.Fatal(err)
			}
			if got.Value.Present != tt.present || !got.Value.Value.Equal(tt.value) {
				t.Fatalf("optional time = (%v, %s), want (%v, %s)", got.Value.Present, got.Value.Value, tt.present, tt.value)
			}
		})
	}
}
