package password

import (
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	hash := Hash("fixture-only-password")
	if strings.Contains(hash, "fixture-only-password") {
		t.Fatal("plaintext in hash")
	}
	if ok, err := Verify("fixture-only-password", hash); !ok || err != nil {
		t.Fatal("password did not verify", err)
	}
	if ok, err := Verify("wrong-password", hash); ok || err != nil {
		t.Fatal("wrong password accepted", err)
	}
	if Hash("fixture-only-password") == hash {
		t.Fatal("salt reused")
	}
}
func TestUntrustedPHCParametersAreRejected(t *testing.T) {
	good := Hash("test-only")
	for _, hash := range []string{"plain", strings.Replace(good, "v=19", "v=16", 1), strings.Replace(good, "m=65536", "m=4294967295", 1), strings.Replace(good, "t=3", "t=999", 1), strings.Replace(good, "p=2", "p=256", 1), strings.Replace(good, "m=65536", "m=065536", 1), good + "$extra", strings.Repeat("x", 256)} {
		if ok, err := Verify("test-only", hash); ok || err == nil {
			t.Fatal("malformed or unbounded hash accepted")
		}
	}
}
func BenchmarkVerify(b *testing.B) {
	hash := Hash("benchmark-fixture-only")
	b.ResetTimer()
	for range b.N {
		ok, err := Verify("benchmark-fixture-only", hash)
		if !ok || err != nil {
			b.Fatal("verify failed")
		}
	}
}
