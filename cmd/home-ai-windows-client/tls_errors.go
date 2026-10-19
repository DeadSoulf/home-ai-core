package main

import (
	"crypto/x509"
	"errors"
)

// certificateErrorMessageKey returns a localized UI message key for errors
// caused by TLS certificate verification. This never changes transport
// verification and never turns authentication into a fail-open operation.
func certificateErrorMessageKey(err error) string {
	if err == nil {
		return ""
	}
	var unknownAuthority x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthority) {
		return "tls_unknown_authority"
	}
	var hostname x509.HostnameError
	if errors.As(err, &hostname) {
		return "tls_hostname_mismatch"
	}
	var invalid x509.CertificateInvalidError
	if errors.As(err, &invalid) {
		if invalid.Reason == x509.Expired {
			return "tls_expired"
		}
		return "tls_invalid"
	}
	return ""
}
