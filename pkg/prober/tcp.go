package prober

import (
	"context"
	"net"
	"net/url"
)

// TCPProber executes raw TCP connection checks.
type TCPProber struct{}

// NewTCPProber creates a new TCP prober.
func NewTCPProber() *TCPProber {
	return &TCPProber{}
}

// ProbeTCPTarget attempts to establish a TCP connection to the target.
// It expects the target to have a scheme (e.g., tcp://10.0.0.1:5432).
func (p *TCPProber) ProbeTCPTarget(ctx context.Context, target string) ErrorCategory {
	parsedURL, err := url.Parse(target)
	if err != nil {
		return CategoryUnknown
	}

	var dialer net.Dialer
	
	// DialContext automatically respects the timeout defined in the provided context
	conn, err := dialer.DialContext(ctx, "tcp", parsedURL.Host)
	if err != nil {
		return MapToCategory(err, 0)
	}

	// Close connection immediately after a successful TCP handshake
	_ = conn.Close()

	return ""
}