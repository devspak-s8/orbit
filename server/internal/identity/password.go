// Package identity owns authentication: user accounts, password hashing,
// and session management.
package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters. Kept deliberately conservative; verify() reads the
// parameters back from the stored hash so these can be raised later without
// breaking existing hashes.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // 64 MiB
	argonThreads = 2
	argonKeyLen  = 32
	argonSaltLen = 16
)

// hashPassword returns an encoded Argon2id hash in the form
// $argon2id$v=19$m=65536,t=3,p=2$<salt>$<key>.
func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return encodeArgon2Hash(argonTime, argonMemory, argonThreads, salt, key), nil
}

// verifyPassword reports whether password matches the encoded hash.
// Comparison is constant-time. An empty or malformed hash never matches.
func verifyPassword(password, encoded string) bool {
	params, salt, key, err := decodeArgon2Hash(encoded)
	if err != nil {
		return false
	}
	other := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, uint32(len(key)))
	return subtle.ConstantTimeCompare(key, other) == 1
}

type argon2Params struct {
	time    uint32
	memory  uint32
	threads uint8
}

func encodeArgon2Hash(time, memory uint32, threads uint8, salt, key []byte) string {
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memory, time, threads, b64(salt), b64(key))
}

func decodeArgon2Hash(encoded string) (argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return argon2Params{}, nil, nil, fmt.Errorf("not an argon2id hash")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return argon2Params{}, nil, nil, fmt.Errorf("unsupported argon2 version")
	}
	var p argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("bad argon2 params: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("bad salt encoding: %w", err)
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("bad key encoding: %w", err)
	}
	return p, salt, key, nil
}