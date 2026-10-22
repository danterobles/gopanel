package auth

import (
	"encoding/base64"
	"testing"
)

func TestOpaqueTokenAndCSRF(t *testing.T) {
	token, hash := newToken()
	if len(token) != 43 || hash == [32]byte{} {
		t.Fatal("invalid random token")
	}
	parsed, err := parseToken(token)
	if err != nil || parsed != hash {
		t.Fatal("token hashing mismatch")
	}
	p := Principal{token: token}
	csrf := p.CSRF()
	if len(csrf) != 43 || csrf == token || csrf == base64.RawURLEncoding.EncodeToString(hash[:]) {
		t.Fatal("CSRF must not expose session token or hash")
	}
	next, _ := newToken()
	if next == token || (Principal{token: next}).CSRF() == csrf {
		t.Fatal("rotation failed")
	}
	for _, value := range []string{"", token + "=", token[:42], "!" + token[1:]} {
		if _, err := parseToken(value); err == nil {
			t.Fatal("malformed token accepted")
		}
	}
}
