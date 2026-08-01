package canonicaltime

import (
	"testing"
	"time"
)

func TestNormalizeUTCAndMicroseconds(t *testing.T) {
	zone := time.FixedZone("test", 5*60*60)
	input := time.Date(2026, 8, 1, 12, 34, 56, 987654321, zone)
	want := time.Date(2026, 8, 1, 7, 34, 56, 987654000, time.UTC)
	if got := Normalize(input); !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("Normalize() = %v (%v), want %v (UTC)", got, got.Location(), want)
	}
}

func TestNormalizePreservesZeroAbsence(t *testing.T) {
	if got := Normalize(time.Time{}); !got.IsZero() {
		t.Fatalf("Normalize(zero) = %v, want zero", got)
	}
}
