package prober

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"time"
	"io"
)

// ErrorCategory represents the 6 SRE error types
type ErrorCategory string

const (
	CategoryDNS     ErrorCategory = "dns_error"
	CategoryConnect ErrorCategory = "connection_refused"
	CategoryTLS     ErrorCategory = "tls_error"
	CategoryTimeout ErrorCategory = "timeout"
	CategoryHTTP    ErrorCategory = "http_error"
	CategoryUnknown ErrorCategory = "unknown_error"
)

// Hint returns actionable troubleshooting context for the SRE operator
func (c ErrorCategory) Hint() string {
	switch c {
	case CategoryDNS:
		return "Verify CoreDNS resolution, cluster domain suffixes, or Service name spelling"
	case CategoryConnect:
		return "Target pod might be down, crash-looping, or rejecting connections on specified port"
	case CategoryTLS:
		return "Check TLS certificate validity, SAN coverage, or CA trust store"
	case CategoryTimeout:
		return "Target is slow to respond; check upstream latency, network network policies, or resource limits"
	case CategoryHTTP:
		return "Application responded with 4xx/5xx; check application logs and payload specs"
	case CategoryUnknown:
		fallthrough
	default:
		return "Unclassified network or protocol error; inspect underlying OS/socket errors"
	}
}

// MapToCategory unwraps the error chain and categorizes network/HTTP issues
func MapToCategory(err error, statusCode int) ErrorCategory {
	if err == nil {
		if statusCode >= 400 {
			return CategoryHTTP
		}
		return "" // Success, no error
	}

	// 1. Check for Context Timeout / Deadline Exceeded
	if errors.Is(err, context.DeadlineExceeded) {
		return CategoryTimeout
	}

	// 2. Check for TLS / Certificate errors
	var certErr *x509.CertificateInvalidError
	var unknownAuthErr *x509.UnknownAuthorityError
	var recErr tls.RecordHeaderError
	if errors.As(err, &certErr) || errors.As(err, &unknownAuthErr) || errors.As(err, &recErr) {
		return CategoryTLS
	}

	// 3. Unwrap network-specific errors (DNS vs Connect vs Network Timeout)
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return CategoryDNS
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Timeout() {
			return CategoryTimeout
		}
		return CategoryConnect
	}

	return CategoryUnknown
}

type HTTPProber struct {
	client *http.Client
}

// NewHTTPProber creates a new prober with sensible defaults
func NewHTTPProber(client *http.Client) *HTTPProber {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &HTTPProber{client: client}
}

// ProbeHTTPTarget executes the HTTP probe and maps the result to an SRE category
func (p *HTTPProber) ProbeHTTPTarget(ctx context.Context, target string) ErrorCategory {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return MapToCategory(err, 0)
	}

	resp, err := p.client.Do(req)
	var statusCode int
	if resp != nil {
		statusCode = resp.StatusCode
		// Copy remaining body to io.Discard and close to enable TCP connection reuse
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	return MapToCategory(err, statusCode)
}