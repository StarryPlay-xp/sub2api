package wgtunnel

import (
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
)

// LoadSecretBox reads a stable, deployment-owned key. Missing or unreadable keys
// never cause a replacement key to be generated; existing nodes must stay blocked.
// The file contains exactly 32 bytes encoded as 64 hexadecimal characters.
func LoadSecretBox(path string) (*SecretBox, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("WireGuard encryption key file is not configured")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("WireGuard encryption key file cannot be opened")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("WireGuard encryption key must be a regular owner-only file")
	}
	data, err := io.ReadAll(io.LimitReader(f, 130))
	if err != nil {
		return nil, errors.New("WireGuard encryption key cannot be read")
	}
	defer clear(data)
	raw := strings.TrimSpace(string(data))
	if len(raw) != 64 {
		return nil, errors.New("WireGuard encryption key must contain 64 hexadecimal characters")
	}
	key, err := hex.DecodeString(raw)
	if err != nil {
		return nil, errors.New("WireGuard encryption key is invalid")
	}
	defer clear(key)
	return NewSecretBox(key)
}
