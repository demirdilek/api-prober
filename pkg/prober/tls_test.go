package prober

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"
)

func generateTestTLSServer(t *testing.T) (*net.Listener, string) {
	// Generate a temporary RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	// Create a self-signed certificate valid for 30 days
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(30 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	cert := tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  privateKey,
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}

	tlsListener := tls.NewListener(listener, &tls.Config{
		Certificates: []tls.Certificate{cert},
	})

	go func() {
		for {
			conn, err := tlsListener.Accept()
			if err != nil {
				return
			}
			// Handle the connection in a separate goroutine and force the handshake
			go func(c net.Conn) {
				defer c.Close()
				if tlsConn, ok := c.(*tls.Conn); ok {
					_ = tlsConn.Handshake()
				}
			}(conn)
		}
	}()

	return &listener, "tls://" + listener.Addr().String()
}

func TestTLSProber_ProbeTLSTarget(t *testing.T) {
	listener, validTarget := generateTestTLSServer(t)
	defer (*listener).Close()

	prober := NewTLSProber(&tls.Config{
		InsecureSkipVerify: true,
	})

	tests := []struct {
		name     string
		target   string
		expected ErrorCategory
	}{
		{
			name:     "Successful TLS handshake",
			target:   validTarget,
			expected: "",
		},
		{
			name:     "Connection refused for TLS",
			target:   "tls://127.0.0.1:59843",
			expected: CategoryConnect,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			got := prober.ProbeTLSTarget(ctx, tt.target)
			if got != tt.expected {
				t.Errorf("ProbeTLSTarget() = %v, want %v", got, tt.expected)
			}
		})
	}
}