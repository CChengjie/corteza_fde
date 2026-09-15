package types

import (
	"testing"
	"time"
)

func TestCity311RetentionUntil(t *testing.T) {
	created := time.Date(2026, time.February, 28, 12, 0, 0, 0, time.UTC)
	want := time.Date(2033, time.February, 28, 12, 0, 0, 0, time.UTC)
	if got := City311RetentionUntil(created); !got.Equal(want) {
		t.Fatalf("retention deadline = %s, want %s", got, want)
	}
	if got := City311RetentionUntil(time.Time{}); !got.IsZero() {
		t.Fatalf("zero creation time should produce zero deadline, got %s", got)
	}
}
