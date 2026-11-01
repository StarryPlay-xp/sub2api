package wgtunnel

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/imroc/req/v3"
	"golang.org/x/net/http2"
)

func testTLS(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "WG isolated test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("10.77.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	cert, err := tls.X509KeyPair(certPEM, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(certPEM)
	return cert, roots
}

func TestTunnelProtocols(t *testing.T) {
	client, server := pairedRuntimes(t)
	cert, roots := testTLS(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(r.Proto)) })
	mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: first\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()
		typ, data, err := c.Read(r.Context())
		if err == nil {
			_ = c.Write(r.Context(), typ, data)
		}
	})
	listener, err := server.network.ListenTCPAddrPort(netip.MustParseAddrPort("10.77.0.1:8443"))
	if err != nil {
		t.Fatal(err)
	}
	hs := &http.Server{Handler: mux, TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}}
	if err = http2.ConfigureServer(hs, &http2.Server{}); err != nil { //nolint:staticcheck // Exercise the same HTTP/2 adapter as the supported transport.
		t.Fatal(err)
	}
	go func() { _ = hs.Serve(tls.NewListener(listener, hs.TLSConfig)) }()
	t.Cleanup(func() { _ = hs.Close() })
	transport := &http.Transport{Proxy: nil, DialContext: client.DialContext, TLSClientConfig: &tls.Config{RootCAs: roots}, ForceAttemptHTTP2: true}
	t.Cleanup(transport.CloseIdleConnections)
	hc := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	t.Run("HTTP2", func(t *testing.T) {
		r, err := hc.Get("https://10.77.0.1:8443/")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = r.Body.Close() }()
		if r.ProtoMajor != 2 {
			t.Fatal("HTTP/2 was not negotiated")
		}
		if _, err = io.Copy(io.Discard, r.Body); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("SSE cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://10.77.0.1:8443/stream", nil)
		r, err := hc.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = r.Body.Close() }()
		line, err := bufio.NewReader(r.Body).ReadString('\n')
		if err != nil || line != "data: first\n" {
			t.Fatal("stream did not flush")
		}
		cancel()
		if _, err = io.Copy(io.Discard, r.Body); err == nil {
			t.Fatal("canceled stream continued")
		}
	})
	t.Run("WebSocket", func(t *testing.T) {
		wsTransport := transport.Clone()
		wsTransport.ForceAttemptHTTP2 = false
		wsTransport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
		defer wsTransport.CloseIdleConnections()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c, _, err := websocket.Dial(ctx, "wss://10.77.0.1:8443/ws", &websocket.DialOptions{HTTPClient: &http.Client{Transport: wsTransport}})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = c.CloseNow() }()
		if err = c.Write(ctx, websocket.MessageText, []byte("echo")); err != nil {
			t.Fatal(err)
		}
		_, data, err := c.Read(ctx)
		if err != nil || string(data) != "echo" {
			t.Fatal("WebSocket echo failed")
		}
	})
	t.Run("req browser fingerprint HTTP2", func(t *testing.T) {
		c := req.C().SetTimeout(5 * time.Second).ImpersonateFirefox().EnableForceHTTP2().SetDial(client.DialContext).SetTLSClientConfig(&tls.Config{RootCAs: roots})
		if err := ConfigureReqClient(c, client.DialContext); err != nil {
			t.Fatal(err)
		}
		defer c.GetTransport().CloseIdleConnections()
		r, err := c.R().Get("https://10.77.0.1:8443/")
		if err != nil {
			t.Fatal(err)
		}
		if r.ProtoMajor != 2 || r.String() != "HTTP/2.0" {
			t.Fatal("browser client did not use HTTP/2 over tunnel")
		}
	})
}
