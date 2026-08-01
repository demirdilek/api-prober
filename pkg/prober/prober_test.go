package prober

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"testing"
)

func TestMapToCategory(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		want       ErrorCategory
	}{
		{
			name:       "Success 200 OK",
			err:        nil,
			statusCode: 200,
			want:       "",
		},
		{
			name:       "HTTP 500 Server Error",
			err:        nil,
			statusCode: 500,
			want:       CategoryHTTP,
		},
		{
			name:       "Context Timeout Exceeded",
			err:        context.DeadlineExceeded,
			statusCode: 0,
			want:       CategoryTimeout,
		},
		{
			name:       "DNS Resolution Failure",
			err:        &net.DNSError{IsNotFound: true},
			statusCode: 0,
			want:       CategoryDNS,
		},
		{
			name:       "Invalid TLS Certificate",
			err:        &x509.CertificateInvalidError{},
			statusCode: 0,
			want:       CategoryTLS,
		},
		{
			name:       "Unknown Generic Error",
			err:        errors.New("something went wrong"),
			statusCode: 0,
			want:       CategoryUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapToCategory(tt.err, tt.statusCode)
			if got != tt.want {
				t.Errorf("MapToCategory() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrorCategory_Hint(t *testing.T) {
	tests := []struct {
		category  ErrorCategory
		wantEmpty bool
	}{
		{category: CategoryDNS, wantEmpty: false},
		{category: CategoryConnect, wantEmpty: false},
		{category: CategoryTLS, wantEmpty: false},
		{category: CategoryTimeout, wantEmpty: false},
		{category: CategoryHTTP, wantEmpty: false},
		{category: CategoryUnknown, wantEmpty: false},
		{category: ErrorCategory("invalid_category"), wantEmpty: false},
	}

	for _, tt := range tests {
		t.Run(string(tt.category), func(t *testing.T) {
			got := tt.category.Hint()
			if (got == "") != tt.wantEmpty {
				t.Errorf("ErrorCategory.Hint() returned empty string for %v", tt.category)
			}
		})
	}
}
