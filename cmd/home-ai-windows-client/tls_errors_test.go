package main

import (
	"crypto/x509"
	"errors"
	"fmt"
	"testing"
)

func TestCertificateErrorMessageKey(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{err: nil, want: ""},
		{err: errors.New("password is invalid"), want: ""},
		{err: fmt.Errorf("login: %w", x509.UnknownAuthorityError{}), want: "tls_unknown_authority"},
		{err: fmt.Errorf("login: %w", x509.HostnameError{Host: "homeaicore.ru", Certificate: &x509.Certificate{}}), want: "tls_hostname_mismatch"},
		{err: fmt.Errorf("login: %w", x509.CertificateInvalidError{Reason: x509.Expired}), want: "tls_expired"},
		{err: fmt.Errorf("login: %w", x509.CertificateInvalidError{Reason: x509.IncompatibleUsage}), want: "tls_invalid"},
	}
	for _, tc := range cases {
		got := certificateErrorMessageKey(tc.err)
		if got != tc.want {
			t.Errorf("certificateErrorMessageKey(%v)=%q, want %q", tc.err, got, tc.want)
		}
	}
}
