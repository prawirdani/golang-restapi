package http

import (
	"encoding/json"
	"testing"

	"github.com/prawirdani/golang-restapi/internal/ports/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBodyEnvelope pins the wire contract of the success envelope: it is sparse
// (only fields holding a value are emitted) and an empty envelope is "{}".
//
// Byte equality is asserted rather than JSON equality because the ETag
// middleware hashes the encoded body, so key order and omitted fields are part
// of the contract. The pointer/value subtests guard a regression we already hit
// once: the envelope used to carry a pointer-receiver MarshalJSON, which made
// the wire format depend on the caller remembering "&".
func TestBodyEnvelope(t *testing.T) {
	// Meta is the whole query metadata. The body only cares that it is omitted
	// while unset, so the instantiation used here is deliberately a plain one.
	pagination := repository.PaginationMeta{Page: 1, Limit: 20, Total: 42, TotalPages: 3}

	meta := repository.QueryMeta[map[string][]string]{
		Filter:     map[string][]string{"role": {"admin"}},
		Pagination: pagination,
	}

	tests := []struct {
		name string
		body Body
		want string
	}{
		{
			name: "message only",
			body: Body{Message: "user updated!"},
			want: `{"message":"user updated!"}`,
		},
		{
			name: "data only",
			body: Body{Data: []string{"a", "b"}},
			want: `{"data":["a","b"]}`,
		},
		{
			name: "empty envelope",
			body: Body{},
			want: `{}`,
		},
		{
			name: "with query metadata",
			body: Body{Data: []string{"a"}, Meta: meta},
			want: `{"data":["a"],"meta":{"filter":{"role":["admin"]},"pagination":{"page":1,"limit":20,"total":42,"total_pages":3}}}`,
		},
		{
			// The metadata itself is sparse: a field the query did not use is
			// dropped, so pagination-only metadata carries no filter or sort.
			name: "metadata without filter",
			body: Body{Data: []string{"a"}, Meta: repository.QueryMeta[map[string][]string]{Pagination: pagination}},
			want: `{"data":["a"],"meta":{"pagination":{"page":1,"limit":20,"total":42,"total_pages":3}}}`,
		},
		{
			name: "metadata with sort",
			body: Body{Data: []string{"a"}, Meta: repository.QueryMeta[map[string][]string]{
				Sort:       repository.Sorting{By: "created_at", Order: "DESC"},
				Pagination: pagination,
			}},
			want: `{"data":["a"],"meta":{"sort":{"by":"created_at","order":"DESC"},"pagination":{"page":1,"limit":20,"total":42,"total_pages":3}}}`,
		},
		{
			// omitempty on an "any" field only drops an unset field: a typed
			// nil stored in the interface is not empty. Pinned so the caveat
			// documented on Body stays true.
			name: "typed nil data is still emitted",
			body: Body{Data: []string(nil)},
			want: `{"data":null}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(&tt.body)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))

			value, err := json.Marshal(tt.body)
			require.NoError(t, err)
			assert.Equal(t, string(got), string(value), "value and pointer must marshal identically")
		})
	}
}
