// Package canonicaltime owns FeatureForge's persisted timestamp precision.
//
// PostgreSQL stores timestamps at microsecond precision.  Normalizing before
// construction keeps the memory and PostgreSQL adapters observationally
// equivalent and makes command replay independent of adapter round-trips.
package canonicaltime

import "time"

// Normalize converts a present timestamp to UTC and truncates it to the
// governed microsecond precision.  The zero value remains the absence marker
// used by optional command fields.
func Normalize(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC().Truncate(time.Microsecond)
}

// Equal reports canonical timestamp equality.
func Equal(left, right time.Time) bool {
	return Normalize(left).Equal(Normalize(right))
}
