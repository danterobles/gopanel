package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

var ErrHash = errors.New("invalid password hash")

func Hash(value string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	key := argon2.IDKey([]byte(value), salt, 3, 65536, 2, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=2$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

// Verify bounds every PHC parameter before allocating memory or deriving a key.
func Verify(value, encoded string) (bool, error) {
	if len(value) > 1024 || len(encoded) > 255 {
		return false, ErrHash
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, ErrHash
	}
	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return false, ErrHash
	}
	numbers := make([]uint32, 3)
	for i, prefix := range []string{"m=", "t=", "p="} {
		if !strings.HasPrefix(params[i], prefix) {
			return false, ErrHash
		}
		raw := strings.TrimPrefix(params[i], prefix)
		v, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || raw != strconv.FormatUint(v, 10) {
			return false, ErrHash
		}
		numbers[i] = uint32(v)
	}
	m, t, p := numbers[0], numbers[1], numbers[2]
	if m < 8192 || m > 131072 || t < 1 || t > 6 || p < 1 || p > 4 {
		return false, ErrHash
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return false, ErrHash
	}
	expected, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(expected) < 16 || len(expected) > 64 {
		return false, ErrHash
	}
	actual := argon2.IDKey([]byte(value), salt, t, m, uint8(p), uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}
