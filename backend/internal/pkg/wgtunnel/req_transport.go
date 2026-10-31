package wgtunnel

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"github.com/imroc/req/v3"
)

// ConfigureReqClient must be called after impersonation and before publishing a
// client to a pool. req's forced HTTP/2 path does not honor DialContext, so both
// TCP and TLS hooks must be explicit. No environment proxy or HTTP/3 may escape
// the chosen route. Browser TLS and HTTP/2 fingerprint settings are preserved.
func ConfigureReqClient(c *req.Client, dial DialContext) error {
	if c == nil || dial == nil {
		return errors.New("an explicit client and route are required")
	}
	tr := c.GetTransport()
	handshake := tr.TLSHandshakeContext
	tr.Proxy = nil
	c.DisableHTTP3().SetDial(dial).SetDialTLS(func(ctx context.Context, network, addr string) (net.Conn, error) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		raw, err := dial(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		if handshake != nil {
			secured, _, err := handshake(ctx, addr, raw)
			if err != nil {
				raw.Close()
				return nil, err
			}
			if secured == nil {
				raw.Close()
				return nil, errors.New("TLS handshake returned no connection")
			}
			return secured, nil
		}
		cfg := c.GetTLSClientConfig().Clone()
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			raw.Close()
			return nil, err
		}
		cfg.ServerName = host
		cfg.NextProtos = []string{"h2", "http/1.1"}
		secured := tls.Client(raw, cfg)
		if err = secured.HandshakeContext(ctx); err != nil {
			raw.Close()
			return nil, err
		}
		return secured, nil
	})
	return nil
}
