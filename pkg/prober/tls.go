package prober

import (
	"context"
	"crypto/tls"
	"net/url"
	"time"
)

// TLSProber executes TLS handshake and certificate expiry checks.
type TLSProber struct {
	tlsConfig *tls.Config
}

// NewTLSProber creates a new TLS prober. Optionally accepts a custom config.
func NewTLSProber(cfg *tls.Config) *TLSProber {
	if cfg == nil {
		// Secure default for production
		cfg = &tls.Config{}
	}
	return &TLSProber{
		tlsConfig: cfg,
	}
}

func (p *TLSProber) ProbeTLSTarget(ctx context.Context, target string) ErrorCategory {
	parsedURL, err := url.Parse(target)
	if err != nil {
		return CategoryUnknown
	
	}
	// Clone the base config so concurrent probes don't cause data races, 
	// and dynamically inject the SNI ServerName.
	cfg := p.tlsConfig.Clone()
	cfg.ServerName = parsedURL.Hostname()

	// Use the injected configuration
	dialer := &tls.Dialer{
		Config: p.tlsConfig,
	}

	conn, err := dialer.DialContext(ctx, "tcp", parsedURL.Host)
	if err != nil {
		return MapToCategory(err, 0)
	}
	defer conn.Close()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return CategoryTLS
	}

	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) > 0 {
		cert := state.PeerCertificates[0]
		daysUntilExpiry := time.Until(cert.NotAfter).Hours() / 24.0
		TLSCertExpiryGauge.WithLabelValues(target).Set(daysUntilExpiry)
	}

	return ""
}