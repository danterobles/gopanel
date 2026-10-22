package id

import (
	"regexp"
	"testing"
)

func TestUUIDv4(t *testing.T) {
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seen := map[string]bool{}
	for range 1000 {
		value := New()
		if !pattern.MatchString(value) || seen[value] {
			t.Fatal("invalid or duplicate UUID")
		}
		seen[value] = true
	}
}

func TestCanonicalUUIDValidation(t *testing.T) {
	for _, value := range []string{New(), "00000000-0000-4000-8000-000000000000", "ffffffff-ffff-4fff-bfff-ffffffffffff"} {
		if !Valid(value) {
			t.Errorf("valid UUID rejected: %s", value)
		}
	}
	for _, value := range []string{"", "not-a-uuid", "😀", "FFFFFFFF-FFFF-4FFF-BFFF-FFFFFFFFFFFF", "00000000-0000-1000-8000-000000000000", "00000000-0000-4000-c000-000000000000", "00000000_0000-4000-8000-000000000000", "00000000-0000-4000-8000-000000000000x"} {
		if Valid(value) {
			t.Errorf("malformed UUID accepted: %s", value)
		}
	}
}
