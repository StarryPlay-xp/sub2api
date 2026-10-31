package wgtunnel

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func exampleConfig() string {
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))
	return "[Interface]\nPrivateKey = " + key + "\nAddress = 10.77.0.2/32\nDNS = 10.77.0.1\n[Peer]\nPublicKey = " + key + "\nEndpoint = vpn.example:51820\nAllowedIPs = 0.0.0.0/0\nPersistentKeepalive = 25\n"
}
func TestParseClientConfiguration(t *testing.T) {
	c, e := Parse(exampleConfig())
	if e != nil {
		t.Fatal(e)
	}
	if c.MTU != 1280 || c.Keepalive != 25 || len(c.Addresses) != 1 {
		t.Fatal("incorrect parsed configuration")
	}
	if _, e = json.Marshal(c); e == nil {
		t.Fatal("secret configuration must not serialize")
	}
	if !strings.Contains(fmt.Sprintf("%#v", c), "redacted") {
		t.Fatal("debug output must redact")
	}
}
func TestRejectUnsafeOrAmbiguousConfiguration(t *testing.T) {
	for name, raw := range map[string]string{
		"scripts":          strings.Replace(exampleConfig(), "[Peer]", "PostUp = secret-command\n[Peer]", 1),
		"multiple peers":   exampleConfig() + "[Peer]\n",
		"missing DNS":      strings.Replace(exampleConfig(), "DNS = 10.77.0.1\n", "", 1),
		"unrouted DNS":     strings.Replace(exampleConfig(), "0.0.0.0/0", "192.0.2.0/24", 1),
		"duplicate secret": strings.Replace(exampleConfig(), "[Peer]", "PrivateKey = private-material\n[Peer]", 1),
		"bad key":          strings.Replace(exampleConfig(), "PrivateKey = ", "PrivateKey = private-material", 1),
		"unknown key":      exampleConfig() + "private-material = value\n",
		"oversized":        strings.Repeat("private-material", MaxConfigBytes),
	} {
		t.Run(name, func(t *testing.T) {
			_, e := Parse(raw)
			if e == nil {
				t.Fatal("expected rejection")
			}
			if strings.Contains(e.Error(), "private-material") || strings.Contains(e.Error(), "secret-command") {
				t.Fatal("error leaked input")
			}
		})
	}
}
func TestSecretBoundToNode(t *testing.T) {
	box, e := NewSecretBox([]byte(strings.Repeat("k", 32)))
	if e != nil {
		t.Fatal(e)
	}
	c, e := box.Seal(10, exampleConfig())
	if e != nil {
		t.Fatal(e)
	}
	p, e := box.Open(10, c)
	if e != nil || p != exampleConfig() {
		t.Fatal("round trip failed")
	}
	if _, e = box.Open(11, c); e == nil {
		t.Fatal("cross-node ciphertext accepted")
	}
	bad := c[:len(c)-4] + "AAAA"
	if _, e = box.Open(10, bad); e == nil {
		t.Fatal("tampered ciphertext accepted")
	}
}

func TestKeyFileRequiresStableRestrictedKey(t *testing.T) {
	p := filepath.Join(t.TempDir(), "master.key")
	if _, err := LoadSecretBox(p); err == nil {
		t.Fatal("missing key accepted")
	}
	if err := os.WriteFile(p, []byte(strings.Repeat("ab", 32)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	box, err := LoadSecretBox(p)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := box.Seal(1, exampleConfig())
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadSecretBox(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reloaded.Open(1, ciphertext); err != nil {
		t.Fatal("restart lost encryption identity")
	}
	if err = os.Chmod(p, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadSecretBox(p); err == nil {
		t.Fatal("world-readable key accepted")
	}
}
