package audit

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/ports/repository"
)

// auth.register
// auth.revoke-session
// auth.complete-registration
// auth.change-password
// auth.revoke-user-sessions
// auth.password-recovery-request
// user.delete
// auth.logout
// user.delete-profile-picture
// auth.login
// user.update
// user.change-profile-picture

// session
// registration_token
// user

type Filter struct {
	Entity []string `json:"entity,omitempty,omitzero" query:"entity"`
	// Actor is a free-text term matched against an entry's actor: a full UUID is
	// matched exactly against actor_id, which stays on the actor index, and
	// anything else is matched against the actor's name with ILIKE. A partial id
	// therefore searches names, not ids.
	Actor string `json:"actor,omitempty" query:"actor"`
	repository.DateRange
}

type Search struct {
	Filter
	repository.Pagination
	repository.Sorting
	meta repository.QueryMeta[Filter]
}

var sortFields = map[string]string{
	"created_at": "e.created_at",
}

func (s *Search) ApplyFilter(q repository.Query) {
	var entities []string

	for _, e := range s.Entity {
		if name, ok := ParseEntity(e); ok {
			entities = append(entities, name)
		}
	}

	// Unknown names are dropped, so they also disappear from the echoed filter.
	s.Entity = entities

	q.WhereIn("e.entity", s.Entity)

	if from, to, ok := s.Bounds(); ok {
		q.WhereBetween("e.created_at", from, to)
	}

	s.Actor = strings.TrimSpace(s.Actor)

	if s.Actor == "" {
		return
	}

	if id, err := uuid.Parse(s.Actor); err == nil {
		q.WhereIn("e.actor_id", []uuid.UUID{id})
		return
	}

	q.WhereILike("u.name", "%"+escapeLike(s.Actor)+"%")
}

// escapeLike neutralises LIKE wildcards so searching for "a_b" matches that
// literal text instead of any character; PostgreSQL's default LIKE escape
// character is a backslash. The pattern is still passed as a bound argument.
func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

// ApplySort implements [repository.Sorter]
func (s *Search) ApplySort(q repository.Query) {
	s.Sorting.ApplySort(q, sortFields)
}

func (s *Search) SetMeta(total int) {
	s.meta = repository.QueryMeta[Filter]{
		Filter:     s.Filter, // applied only sanitized by ApplyFilter
		Sort:       s.Sorting,
		Pagination: s.PageMeta(total),
	}
}

func (s Search) Meta() repository.QueryMeta[Filter] {
	return s.meta
}

// Actor is the user an entry is attributed to. EntryWithActor leaves it nil for
// entries written by a system actor, which have no user row.
type Actor struct {
	ID   uuid.UUID `json:"id"   db:"id"`
	Name string    `json:"name" db:"name"`
}

type EntryWithActor struct {
	ID        int            `db:"id"         json:"id"`
	Actor     *Actor         `db:"actor"      json:"actor"`
	Action    Action         `db:"action"     json:"action"`
	Entity    string         `db:"entity"     json:"entity"`
	EntityID  string         `db:"entity_id"  json:"entity_id"`
	Prev      any            `db:"prev"       json:"prev"`
	Next      any            `db:"next"       json:"next"`
	Meta      map[string]any `db:"meta"       json:"meta"`
	CreatedAt time.Time      `db:"created_at" json:"created_at"`
}
