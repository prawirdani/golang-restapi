package audit

import "strings"

// validEntities are the logical records audit entries are recorded under — the
// values services pass as [Entry.Entity].
var validEntities = map[string]struct{}{
	"registration_token": {},
	"session":            {},
	"user":               {},
}

// ParseEntity canonicalises an entity name and reports whether the domain
// records entries under it. Unknown names are rejected so callers can drop them,
// the way the other filters drop values the domain does not recognise.
func ParseEntity(value string) (string, bool) {
	name := strings.ToLower(strings.TrimSpace(value))

	if _, ok := validEntities[name]; !ok {
		return "", false
	}

	return name, true
}
