package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestDateRange_Bounds pins the half-open interval semantics and the in-place
// canonical echo: the struct must keep describing the user's input, because the
// response metadata echoes it, while the returned bounds are what the query
// filters on.
func TestDateRange_Bounds(t *testing.T) {
	jan1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	jan2 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	jan3 := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
	feb1 := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	mar15 := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	mar16 := time.Date(2024, 3, 16, 0, 0, 0, 0, time.UTC)

	// A caller's own offset travels inside the value, so the day it selects has
	// to be read in that offset rather than in UTC.
	plus7 := time.FixedZone("", 7*60*60)

	tests := []struct {
		name     string
		in       DateRange
		wantFrom time.Time
		wantTo   time.Time
		wantOK   bool
		want     DateRange // canonical echo
	}{
		{
			name:     "a bare date covers the whole day",
			in:       DateRange{Date: "2024-01-02"},
			wantFrom: jan2,
			wantTo:   jan3,
			wantOK:   true,
			want:     DateRange{Date: "2024-01-02"},
		},
		{
			name:     "a UTC timestamp date starts a 24-hour window at the instant",
			in:       DateRange{Date: "2024-01-02T17:00:00.000Z"},
			wantFrom: time.Date(2024, 1, 2, 17, 0, 0, 0, time.UTC),
			wantTo:   time.Date(2024, 1, 3, 17, 0, 0, 0, time.UTC),
			wantOK:   true,
			want:     DateRange{Date: "2024-01-02T17:00:00Z"},
		},
		{
			name:     "an offset timestamp date keeps its offset on both bounds",
			in:       DateRange{Date: "2024-01-02T17:00:00+07:00"},
			wantFrom: time.Date(2024, 1, 2, 17, 0, 0, 0, plus7),
			wantTo:   time.Date(2024, 1, 3, 17, 0, 0, 0, plus7),
			wantOK:   true,
			want:     DateRange{Date: "2024-01-02T17:00:00+07:00"},
		},
		{
			name:     "fractional seconds and the offset survive the echo",
			in:       DateRange{From: "2024-01-31T12:30:00.123456+07:00"},
			wantFrom: time.Date(2024, 1, 31, 5, 30, 0, 123456000, time.UTC),
			wantOK:   true,
			want:     DateRange{From: "2024-01-31T12:30:00.123456+07:00"},
		},
		{
			name:     "from and to bare dates include the whole to day",
			in:       DateRange{From: "2024-01-01", To: "2024-01-31"},
			wantFrom: jan1,
			wantTo:   feb1,
			wantOK:   true,
			want:     DateRange{From: "2024-01-01", To: "2024-01-31"},
		},
		{
			name:     "from only is open on the upper side",
			in:       DateRange{From: "2024-01-01"},
			wantFrom: jan1,
			wantOK:   true,
			want:     DateRange{From: "2024-01-01"},
		},
		{
			name:   "to only is open on the lower side",
			in:     DateRange{To: "2024-01-31"},
			wantTo: feb1,
			wantOK: true,
			want:   DateRange{To: "2024-01-31"},
		},
		{
			name:   "an RFC3339 to is used as the exclusive instant, not shifted",
			in:     DateRange{To: "2024-01-31T12:30:00Z"},
			wantTo: time.Date(2024, 1, 31, 12, 30, 0, 0, time.UTC),
			wantOK: true,
			want:   DateRange{To: "2024-01-31T12:30:00Z"},
		},
		{
			name:     "an RFC3339 offset is preserved as an instant",
			in:       DateRange{From: "2024-01-31T12:30:00+07:00"},
			wantFrom: time.Date(2024, 1, 31, 5, 30, 0, 0, time.UTC),
			wantOK:   true,
			want:     DateRange{From: "2024-01-31T12:30:00+07:00"},
		},
		{
			name:     "date wins over from and to, which are cleared",
			in:       DateRange{Date: "2024-03-15", From: "2020-01-01", To: "2020-02-01"},
			wantFrom: mar15,
			wantTo:   mar16,
			wantOK:   true,
			want:     DateRange{Date: "2024-03-15"},
		},
		{
			name: "malformed values are dropped and add no bound",
			in:   DateRange{Date: "nope", From: "2020-13-40", To: "garbage"},
			want: DateRange{},
		},
		{
			name: "an all-empty range is not ok",
			in:   DateRange{},
			want: DateRange{},
		},
		{
			name:     "the echo keeps each side's own input form",
			in:       DateRange{From: "2024-01-01", To: "2024-01-31T23:59:59Z"},
			wantFrom: jan1,
			wantTo:   time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC),
			wantOK:   true,
			want:     DateRange{From: "2024-01-01", To: "2024-01-31T23:59:59Z"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in

			from, to, ok := in.Bounds()

			assert.Equal(t, tt.wantOK, ok)
			assert.True(t, tt.wantFrom.Equal(from), "from: want %v, got %v", tt.wantFrom, from)
			assert.True(t, tt.wantTo.Equal(to), "to: want %v, got %v", tt.wantTo, to)
			assert.Equal(t, tt.want, in)
		})
	}
}
