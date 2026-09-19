package redis

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRevocationTTL(t *testing.T) {
	tests := []struct {
		name   string
		jwtTTL time.Duration
		want   time.Duration
	}{
		{
			name:   "normal ttl adds skew",
			jwtTTL: 15 * time.Minute,
			want:   15*time.Minute + revocationSkew,
		},
		{
			name:   "zero ttl yields zero",
			jwtTTL: 0,
			want:   0,
		},
		{
			name:   "negative ttl yields zero",
			jwtTTL: -time.Minute,
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, revocationTTL(tt.jwtTTL))
		})
	}
}

func TestRevokedByWatermark(t *testing.T) {
	watermark := time.Unix(0, int64(time.Hour))
	nano := watermark.UnixNano()

	tests := []struct {
		name     string
		issuedAt time.Time
		want     bool
	}{
		{
			name:     "issued exactly at watermark is revoked",
			issuedAt: watermark,
			want:     true,
		},
		{
			name:     "issued just inside skew margin is revoked",
			issuedAt: watermark.Add(revocationSkew - time.Nanosecond),
			want:     true,
		},
		{
			name:     "issued exactly at skew margin is revoked",
			issuedAt: watermark.Add(revocationSkew),
			want:     true,
		},
		{
			name:     "issued just past skew margin is kept",
			issuedAt: watermark.Add(revocationSkew + time.Nanosecond),
			want:     false,
		},
		{
			name:     "issued well before watermark is revoked",
			issuedAt: watermark.Add(-time.Hour),
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, revokedByWatermark(tt.issuedAt, nano))
		})
	}
}

func TestDecide(t *testing.T) {
	watermark := time.Unix(0, int64(time.Hour))
	userMarker := strconv.FormatInt(watermark.UnixNano(), 10)

	tests := []struct {
		name          string
		userMarker    string
		sessionMarker string
		issuedAt      time.Time
		want          bool
		wantErr       bool
	}{
		{
			name:     "no markers is kept",
			issuedAt: watermark,
			want:     false,
		},
		{
			name:          "session marker alone revokes",
			sessionMarker: "1",
			issuedAt:      watermark,
			want:          true,
		},
		{
			name:          "session marker takes precedence over user marker",
			userMarker:    userMarker,
			sessionMarker: "1",
			issuedAt:      watermark,
			want:          true,
		},
		{
			name:       "user watermark at skew boundary revokes",
			userMarker: userMarker,
			issuedAt:   watermark.Add(revocationSkew),
			want:       true,
		},
		{
			name:       "user watermark before skew boundary revokes",
			userMarker: userMarker,
			issuedAt:   watermark.Add(-time.Hour),
			want:       true,
		},
		{
			name:       "user watermark past skew boundary is kept",
			userMarker: userMarker,
			issuedAt:   watermark.Add(revocationSkew + time.Nanosecond),
			want:       false,
		},
		{
			name:       "corrupt user marker errors",
			userMarker: "not-a-number",
			issuedAt:   watermark,
			want:       false,
			wantErr:    true,
		},
		{
			name:       "zero watermark errors",
			userMarker: "0",
			issuedAt:   watermark,
			want:       false,
			wantErr:    true,
		},
		{
			name:       "negative watermark errors",
			userMarker: "-1",
			issuedAt:   watermark,
			want:       false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decide(tt.userMarker, tt.sessionMarker, tt.issuedAt)
			if tt.wantErr {
				assert.Error(t, err)
				assert.False(t, got)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
