package identity

import "testing"

func TestHashPasswordRoundtrip(t *testing.T) {
	hash, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hashPassword: %v", err)
	}
	if !verifyPassword("correct horse battery staple", hash) {
		t.Error("verifyPassword returned false for the correct password")
	}
	if verifyPassword("wrong password", hash) {
		t.Error("verifyPassword returned true for a wrong password")
	}
}

func TestHashPasswordUniqueSalts(t *testing.T) {
	h1, err := hashPassword("same password")
	if err != nil {
		t.Fatal(err)
	}
	h2, err := hashPassword("same password")
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Error("two hashes of the same password are identical; salts are not unique")
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	cases := []string{
		"",
		"not-a-hash",
		"$argon2id$v=999$m=65536,t=3,p=2$c2FsdA$a2V5",
		"$argon2id$v=19$m=65536,t=3,p=2$!!!$a2V5",
		"$bcrypt$v=19$m=65536,t=3,p=2$c2FsdA$a2V5",
	}
	for _, c := range cases {
		if verifyPassword("password", c) {
			t.Errorf("verifyPassword accepted malformed hash %q", c)
		}
	}
}

func TestHashPasswordEncodingRoundtrip(t *testing.T) {
	// The encoded form must decode back to the same parameters.
	hash, err := hashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	params, salt, key, err := decodeArgon2Hash(hash)
	if err != nil {
		t.Fatalf("decodeArgon2Hash: %v", err)
	}
	if params.memory != argonMemory || params.time != argonTime || params.threads != argonThreads {
		t.Errorf("params = %+v, want memory=%d time=%d threads=%d", params, argonMemory, argonTime, argonThreads)
	}
	if len(salt) != argonSaltLen {
		t.Errorf("salt length = %d, want %d", len(salt), argonSaltLen)
	}
	if len(key) != argonKeyLen {
		t.Errorf("key length = %d, want %d", len(key), argonKeyLen)
	}
}

func TestSessionTokenProperties(t *testing.T) {
	raw1, hash1, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	raw2, hash2, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if raw1 == raw2 || hash1 == hash2 {
		t.Error("two session tokens are identical")
	}
	if hashToken(raw1) != hash1 {
		t.Error("hashToken does not match the stored digest")
	}
	if len(hash1) != 64 {
		t.Errorf("token hash length = %d, want 64 (sha256 hex)", len(hash1))
	}
}