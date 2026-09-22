package repository

import "time"

// DateRange filters a timestamp column by a half-open interval [From, To).
// It binds from the query string and sanitises itself in place, so the response
// metadata echoes exactly what was applied, like Sorting and Pagination.
type DateRange struct {
	Date string `query:"date" json:"date,omitempty"`
	From string `query:"from" json:"from,omitempty"`
	To   string `query:"to"   json:"to,omitempty"`
}

// ParseDate reads a calendar date (2006-01-02) as UTC, or an RFC3339 timestamp
// that carries its own offset for a precise instant.
func ParseDate(value string) (time.Time, error) {
	t, _, err := parseBound(value)
	return t, err
}

// Bounds returns the half-open interval the filter asks for and rewrites the
// request fields to their canonical form. ok is false when nothing usable was
// supplied — malformed values are dropped, mirroring how the entity filters drop
// values the domain does not recognise. (Surfacing 422 for a malformed date needs
// a Validate method the handler calls; deliberately not added here.)
//
// A bare calendar date means that whole UTC day. A full timestamp is taken as
// the start of a 24-hour window: the caller works out its own day boundaries and
// sends the exact instant — its offset travels in the value — so the server
// never needs to know a timezone. from/to are used as the instants themselves.
func (d *DateRange) Bounds() (from, to time.Time, ok bool) {
	// date wins over from/to; a malformed date falls through to them.
	if d.Date != "" {
		if t, dateOnly, err := parseBound(d.Date); err == nil {
			d.Date = canonicalDate(t, dateOnly)
			d.From, d.To = "", ""

			// The window starts at the instant the caller sent, so `from` is
			// exactly that value and the echoed metadata describes what the
			// query bound to.
			return t, t.Add(24 * time.Hour), true
		}
		d.Date = ""
	}

	if d.From != "" {
		if t, dateOnly, err := parseBound(d.From); err == nil {
			from = t
			ok = true
			d.From = canonicalDate(t, dateOnly)
		} else {
			d.From = ""
		}
	}

	if d.To != "" {
		if t, dateOnly, err := parseBound(d.To); err == nil {
			to = t
			if dateOnly {
				// A bare `to` date includes that whole day.
				to = t.AddDate(0, 0, 1)
			}
			ok = true
			d.To = canonicalDate(t, dateOnly)
		} else {
			d.To = ""
		}
	}

	return from, to, ok
}

// parseBound parses a bound as a bare calendar date (UTC) or an RFC3339 instant
// that carries its own offset. dateOnly reports which form matched, so the
// canonical echo can keep the user's input form.
func parseBound(value string) (t time.Time, dateOnly bool, err error) {
	if t, err = time.Parse(time.DateOnly, value); err == nil {
		return t, true, nil
	}

	t, err = time.Parse(time.RFC3339, value)
	return t, false, err
}

// canonicalDate renders a parsed bound in the form the input used: a bare date
// stays a bare date, and a timestamp keeps its offset and any fractional second,
// so the echoed metadata describes the same instant the caller sent.
func canonicalDate(t time.Time, dateOnly bool) string {
	if dateOnly {
		return t.Format(time.DateOnly)
	}

	return t.Format(time.RFC3339Nano)
}
