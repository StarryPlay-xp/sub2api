package wgtunnel

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// SecretBox uses a deployment-owned 32-byte key. The key is never stored with
// the database. Associated data prevents moving ciphertext to another node.
type SecretBox struct{ aead cipher.AEAD }

func (*SecretBox) String() string   { return "[WireGuard encryption key redacted]" }
func (*SecretBox) GoString() string { return "[WireGuard encryption key redacted]" }
func (*SecretBox) MarshalJSON() ([]byte, error) {
	return nil, errors.New("WireGuard encryption keys cannot be serialized")
}

func NewSecretBox(key []byte) (*SecretBox, error) {
	if len(key) != 32 {
		return nil, errors.New("WireGuard encryption key must contain 32 bytes")
	}
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	a, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	return &SecretBox{a}, nil
}
func secretAAD(nodeID int64) []byte {
	return []byte(fmt.Sprintf("sub2api/wireguard/config/v1/%d", nodeID))
}
func (s *SecretBox) Seal(nodeID int64, plain string) (string, error) {
	if s == nil {
		return "", errors.New("WireGuard encryption key is unavailable")
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", errors.New("WireGuard encryption failed")
	}
	return base64.StdEncoding.EncodeToString(s.aead.Seal(nonce, nonce, []byte(plain), secretAAD(nodeID))), nil
}
func (s *SecretBox) Open(nodeID int64, encoded string) (string, error) {
	if s == nil {
		return "", errors.New("WireGuard encryption key is unavailable")
	}
	b, e := base64.StdEncoding.DecodeString(encoded)
	if e != nil || len(b) < s.aead.NonceSize() {
		return "", errors.New("WireGuard configuration cannot be decrypted")
	}
	n := s.aead.NonceSize()
	p, e := s.aead.Open(nil, b[:n], b[n:], secretAAD(nodeID))
	if e != nil {
		return "", errors.New("WireGuard configuration cannot be decrypted")
	}
	return string(p), nil
}
