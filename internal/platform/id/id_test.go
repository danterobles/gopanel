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
