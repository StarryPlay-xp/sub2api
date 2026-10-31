// Package wgtunnel provides an isolated WireGuard network, never a host route.
package wgtunnel

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

const MaxConfigBytes = 32 << 10

// Config must never be logged or serialized. Only Summary is safe for an API.
type Config struct {
	privateKey      [32]byte
	publicKey       [32]byte
	presharedKey    [32]byte
	HasPresharedKey bool
	Addresses       []netip.Addr
	DNS             []netip.Addr
	AllowedIPs      []netip.Prefix
	Endpoint        string
	MTU             int
	Keepalive       int
}

func (Config) String() string   { return "[WireGuard configuration redacted]" }
func (Config) GoString() string { return "[WireGuard configuration redacted]" }
func (Config) MarshalJSON() ([]byte, error) {
	return nil, errors.New("WireGuard configuration is secret; use Summary")
}

type Summary struct {
	Addresses       []string `json:"addresses"`
	DNS             []string `json:"dns"`
	AllowedIPs      []string `json:"allowed_ips"`
	Endpoint        string   `json:"endpoint"`
	MTU             int      `json:"mtu"`
	Keepalive       int      `json:"persistent_keepalive"`
	HasPresharedKey bool     `json:"has_preshared_key"`
}

func (c Config) Summary() Summary {
	s := Summary{Endpoint: c.Endpoint, MTU: c.MTU, Keepalive: c.Keepalive, HasPresharedKey: c.HasPresharedKey}
	for _, a := range c.Addresses {
		s.Addresses = append(s.Addresses, a.String())
	}
	for _, a := range c.DNS {
		s.DNS = append(s.DNS, a.String())
	}
	for _, a := range c.AllowedIPs {
		s.AllowedIPs = append(s.AllowedIPs, a.String())
	}
	return s
}

// Parse deliberately implements a strict client subset, not wg-quick scripts.
// Errors contain line numbers and field names, never submitted values.
func Parse(raw string) (Config, error) {
	c := Config{MTU: 1280}
	if len(raw) == 0 || len(raw) > MaxConfigBytes {
		return c, errors.New("WireGuard configuration must contain 1..32768 bytes")
	}
	section := ""
	sections := map[string]bool{}
	seen := map[string]bool{}
	scan := bufio.NewScanner(strings.NewReader(strings.TrimPrefix(raw, "\ufeff")))
	line := 0
	fail := func(field string) (Config, error) {
		return Config{}, fmt.Errorf("invalid or unsupported WireGuard field at line %d (%s)", line, field)
	}
	for scan.Scan() {
		line++
		text := strings.TrimSpace(strings.SplitN(scan.Text(), "#", 2)[0])
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "[") {
			if text != "[Interface]" && text != "[Peer]" {
				return fail("section")
			}
			if sections[text] || (text == "[Peer]" && !sections["[Interface]"]) {
				return fail("section")
			}
			sections[text] = true
			section = text
			continue
		}
		parts := strings.SplitN(text, "=", 2)
		if len(parts) != 2 || section == "" {
			return fail("syntax")
		}
		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		// Never echo an unrecognized key: an input line may itself be a secret.
		valid := (section == "[Interface]" && (key == "PrivateKey" || key == "Address" || key == "DNS" || key == "MTU" || key == "ListenPort")) ||
			(section == "[Peer]" && (key == "PublicKey" || key == "PresharedKey" || key == "Endpoint" || key == "AllowedIPs" || key == "PersistentKeepalive"))
		if !valid {
			return fail("unsupported")
		}
		multi := key == "Address" || key == "DNS" || key == "AllowedIPs"
		if seen[key] && !multi {
			return fail(key)
		}
		seen[key] = true
		switch key {
		case "PrivateKey", "PublicKey", "PresharedKey":
			b, err := base64.StdEncoding.Strict().DecodeString(value)
			if err != nil || len(b) != 32 {
				return fail(key)
			}
			var k [32]byte
			copy(k[:], b)
			if k == [32]byte{} {
				return fail(key)
			}
			switch key {
			case "PrivateKey":
				c.privateKey = k
			case "PublicKey":
				c.publicKey = k
			default:
				c.presharedKey = k
				c.HasPresharedKey = true
			}
		case "Address", "DNS", "AllowedIPs":
			for _, item := range strings.Split(value, ",") {
				item = strings.TrimSpace(item)
				if key == "AllowedIPs" {
					p, err := netip.ParsePrefix(item)
					if err != nil || p.Addr().Is4In6() {
						return fail(key)
					}
					c.AllowedIPs = append(c.AllowedIPs, p.Masked())
					continue
				}
				var a netip.Addr
				if p, err := netip.ParsePrefix(item); err == nil && key == "Address" {
					a = p.Addr()
				} else {
					a, _ = netip.ParseAddr(item)
				}
				if !a.IsValid() || a.IsUnspecified() || a.IsMulticast() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.Zone() != "" || a.Is4In6() {
					return fail(key)
				}
				if key == "Address" {
					c.Addresses = append(c.Addresses, a)
				} else {
					c.DNS = append(c.DNS, a)
				}
			}
		case "MTU":
			v, err := strconv.Atoi(value)
			if err != nil || v < 1280 || v > 1420 {
				return fail(key)
			}
			c.MTU = v
		case "ListenPort":
			if value != "0" {
				return fail(key)
			}
		case "PersistentKeepalive":
			v, err := strconv.Atoi(value)
			if err != nil || v < 0 || v > 65535 {
				return fail(key)
			}
			c.Keepalive = v
		case "Endpoint":
			host, port, err := net.SplitHostPort(value)
			n, perr := strconv.Atoi(port)
			if err != nil || perr != nil || n < 1 || n > 65535 || host == "" || strings.ContainsAny(host, " /\\@?#%\t\r\n") {
				return fail(key)
			}
			c.Endpoint = net.JoinHostPort(host, strconv.Itoa(n))
		}
		if len(c.Addresses) > 8 || len(c.DNS) > 8 || len(c.AllowedIPs) > 128 {
			return fail("address limit")
		}
	}
	if scan.Err() != nil {
		return Config{}, errors.New("invalid WireGuard configuration")
	}
	for _, required := range []string{"PrivateKey", "Address", "DNS", "PublicKey", "Endpoint", "AllowedIPs"} {
		if !seen[required] {
			return Config{}, fmt.Errorf("missing WireGuard field: %s", required)
		}
	}
	for _, addr := range c.DNS {
		if !c.Allows(addr) {
			return Config{}, errors.New("WireGuard DNS address is not covered by Address and AllowedIPs")
		}
	}
	return c, nil
}

func (c Config) Allows(ip netip.Addr) bool {
	family := false
	for _, local := range c.Addresses {
		if local.Is4() == ip.Is4() {
			family = true
			break
		}
	}
	if !family {
		return false
	}
	for _, p := range c.AllowedIPs {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}
