package wgtunnel

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/imroc/req/v3"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Opt-in only. This uses a separately authorized test server and never reads a
// default configuration path or prints key material/public egress addresses.
func TestAuthorizedRemoteExit(t *testing.T) {
	path := os.Getenv("SUB2API_WG_TEST_CONFIG")
	if path == "" {
		t.Skip("remote WG configuration was not explicitly supplied")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal("cannot read test configuration metadata")
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatal("test configuration must not be accessible by group or others")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("cannot read test configuration")
	}
	cfg, err := Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	r, err := Start(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	evidence, err := ProbeWithService(context.Background(), r.DialContext, "ipv4", "icanhazip")
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Source != "route_probe" || evidence.State != "observed" {
		t.Fatal("missing exit evidence")
	}

	expectedPath := os.Getenv("SUB2API_WG_TEST_EXPECTED_EXIT")
	if expectedPath != "" {
		expected, err := os.ReadFile(expectedPath)
		if err != nil {
			t.Fatal("cannot read expected exit")
		}
		if evidence.IP != strings.TrimSpace(string(expected)) {
			t.Fatal("tunnel observation differs from server observation")
		}
	}
	t.Run("TLS fingerprint", func(t *testing.T) {
		dialer := tlsfingerprint.NewDialer(&tlsfingerprint.Profile{Name: "WG-test"}, r.DialContext)
		tr := &http.Transport{Proxy: nil, DialContext: r.DialContext, DialTLSContext: dialer.DialTLSContext}
		defer tr.CloseIdleConnections()
		c := &http.Client{Transport: tr, Timeout: 10 * time.Second}
		resp, err := c.Get("https://icanhazip.com")
		if err != nil {
			t.Fatal("fingerprint TLS through WG failed")
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(io.LimitReader(resp.Body, 128))
		if err != nil || strings.TrimSpace(string(data)) != evidence.IP {
			t.Fatal("fingerprint exit mismatch")
		}
	})
	t.Run("req fingerprint", func(t *testing.T) {
		c := req.C().ImpersonateFirefox().EnableForceHTTP2().SetTimeout(10 * time.Second)
		if err := ConfigureReqClient(c, r.DialContext); err != nil {
			t.Fatal(err)
		}
		defer c.GetTransport().CloseIdleConnections()
		resp, err := c.R().Get("https://icanhazip.com")
		if err != nil {
			t.Fatal("req fingerprint through WG failed")
		}
		if strings.TrimSpace(resp.String()) != evidence.IP {
			t.Fatal("req exit mismatch")
		}
	})
	t.Run("explicit direct evidence", func(t *testing.T) {
		direct, err := ProbeWithService(context.Background(), (&net.Dialer{}).DialContext, "ipv4", "icanhazip")
		if err != nil {
			t.Fatal(err)
		}
		if direct.IP == "" || direct.Source != "route_probe" {
			t.Fatal("missing direct observation")
		}
	})
	c := r.Counts.Snapshot()
	if c.TX == 0 || c.RX == 0 {
		t.Fatal("missing remote traffic counters")
	}
	r.Close()
	if _, err = ProbeWithService(context.Background(), r.DialContext, "ipv4", "icanhazip"); err == nil {
		t.Fatal("revoked route remained usable")
	}
}
