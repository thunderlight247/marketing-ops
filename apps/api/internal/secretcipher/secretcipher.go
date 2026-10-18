// Package secretcipher encrypts and decrypts integration credentials at rest.
//
// Ciphertext layout is nonce || AES-256-GCM sealed data, stored as one bytea in
// the database. The key never touches the database: it is loaded from a
// environment variable (32 bytes, hex encoded). The database stores the key
// version alongside the ciphertext, so rotation is a re-encrypt migration, and
// old rows remain readable as long as their key version is still loaded.
package secretcipher

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// KeyLength is the required AES-256 key size, 32 bytes.
const KeyLength = 32

// Errors are deliberately non-cryptographic in detail: returning cipher or
// padding failure classes avoids giving attackers oracle distinctions.
var (
	ErrBadKey        = errors.New("credential key is not valid")
	ErrBadCiphertext = errors.New("ciphertext is not valid")
)

// LoadKey parses a 32-byte key from its hex-encoded environment form.
func LoadKey(hexEncoded string) ([]byte, error) {
	raw, err := hexDecode(hexEncoded)
	if err != nil {
		return nil, ErrBadKey
	}
	if len(raw) != KeyLength {
		return nil, ErrBadKey
	}
	return raw, nil
}

// Encrypt seals plaintext under key, returning nonce || ciphertext.
func Encrypt(key, plaintext []byte) ([]byte, error) {
	gcm, err := blockCipher(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("random nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt opens nonce || ciphertext produced by Encrypt.
func Decrypt(key, data []byte) ([]byte, error) {
	gcm, err := blockCipher(key)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(data) < ns+gcm.Overhead() {
		return nil, ErrBadCiphertext
	}
	opened, err := gcm.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return nil, ErrBadCiphertext
	}
	return opened, nil
}

func blockCipher(key []byte) (cipher.AEAD, error) {
	if len(key) != KeyLength {
		return nil, ErrBadKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrBadKey
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrBadKey
	}
	return gcm, nil
}

// hexDecode is strict about its input: no surrounding whitespace games.
func hexDecode(s string) ([]byte, error) {
	if strings.ContainsAny(s, " \t\n") {
		return nil, errors.New("not hex")
	}
	return hex.DecodeString(strings.ToLower(s))
}
