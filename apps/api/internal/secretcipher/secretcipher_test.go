package secretcipher

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

func hexKey(t *testing.T, seed byte) []byte {
	t.Helper()
	k := make([]byte, KeyLength)
	for i := range k {
		k[i] = seed
	}
	return k
}

func TestRoundTrip(t *testing.T) {
	key := hexKey(t, 0x2b)
	for _, plaintext := range [][]byte{
		[]byte("sanity-bearer-token"),
		[]byte(""),
		bytes.Repeat([]byte("x"), 4096),
	} {
		sealed, err := Encrypt(key, plaintext)
		if err != nil {
			t.Fatalf("Encrypt(%d bytes): %v", len(plaintext), err)
		}
		if bytes.Contains(sealed, plaintext) && len(plaintext) > 0 {
			t.Errorf("plaintext visible in ciphertext")
		}
		opened, err := Decrypt(key, sealed)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if !bytes.Equal(opened, plaintext) {
			t.Errorf("roundtrip mismatch: %q -> %q", plaintext, opened)
		}
	}
}

func TestSealedOutputIsUniquePerCall(t *testing.T) {
	key := hexKey(t, 0x99)
	a, err := Encrypt(key, []byte("same"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	b, err := Encrypt(key, []byte("same"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Equal(a, b) {
		t.Error("identical inputs produced identical ciphertext (nonce reused?)")
	}
}

func TestDecryptRejectsWrongKeyAndGarbage(t *testing.T) {
	key := hexKey(t, 0x11)
	sealed, err := Encrypt(key, []byte("token"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	other := hexKey(t, 0x22)
	if _, err := Decrypt(other, sealed); err != ErrBadCiphertext {
		t.Errorf("wrong key must be ErrBadCiphertext, got %v", err)
	}
	if _, err := Decrypt(key, []byte("tiny")); err != ErrBadCiphertext {
		t.Errorf("truncated data must be ErrBadCiphertext, got %v", err)
	}
	if _, err := Decrypt(key, nil); err != ErrBadCiphertext {
		t.Errorf("empty data must be ErrBadCiphertext, got %v", err)
	}
}

func TestLoadKeyValidatesSizeAndHex(t *testing.T) {
	good := strings.Repeat("ab", KeyLength)
	if k, err := LoadKey(good); err != nil || len(k) != KeyLength {
		t.Errorf("good key rejected: %v len=%d", err, len(k))
	}
	if _, err := LoadKey(hex.EncodeToString(make([]byte, KeyLength-1))); err != ErrBadKey {
		t.Errorf("short key must be ErrBadKey, got %v", err)
	}
	if _, err := LoadKey("zz"); err != ErrBadKey {
		t.Errorf("non-hex key must be ErrBadKey, got %v", err)
	}
}

func TestEncryptRejectsBadKey(t *testing.T) {
	if _, err := Encrypt(make([]byte, 10), []byte("x")); err != ErrBadKey {
		t.Errorf("short key must be ErrBadKey, got %v", err)
	}
}
