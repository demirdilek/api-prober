package prober

import (
	"context"
	"net/http"
	"time"
	"io"
)

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