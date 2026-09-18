package user

import (
	"fmt"
	"strings"
)

type Gender string

const (
	GenderMale   Gender = "M"
	GenderFemale Gender = "F"
	GenderOther  Gender = "O"
)

// ParseGender canonicalises and validates a client-supplied gender, so filter
// values can be accepted in any casing and unknown ones rejected.
func ParseGender(s string) (Gender, bool) {
	g := Gender(strings.ToUpper(strings.TrimSpace(s)))

	return g, g.IsValid()
}

func (g Gender) IsValid() bool {
	switch g {
	case GenderMale, GenderFemale, GenderOther:
		return true
	default:
		return false
	}
}

// Scan implements [sql.Scanner]
func (g *Gender) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*g = ""
		return nil

	case string:
		*g = Gender(v)

	case []byte:
		*g = Gender(v)

	default:
		return fmt.Errorf("cannot scan %T into Gender", src)
	}

	if !g.IsValid() {
		return fmt.Errorf("invalid gender %q", *g)
	}

	return nil
}
