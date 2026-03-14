package internal

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
)

// IsTLSCertError returns true if the error is caused by a TLS certificate
// verification failure (e.g. unknown authority, expired, wrong host).
func IsTLSCertError(err error) bool {
	if err == nil {
		return false
	}
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return true
	}
	var unknownAuth x509.UnknownAuthorityError
	if errors.As(err, &unknownAuth) {
		return true
	}
	var hostErr x509.HostnameError
	if errors.As(err, &hostErr) {
		return true
	}
	var certInvalid x509.CertificateInvalidError
	if errors.As(err, &certInvalid) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return IsTLSCertError(urlErr.Err)
	}
	return false
}

// TLSCertErrorGuidance returns user-facing advice for resolving TLS
// certificate errors when connecting to the given service.
func TLSCertErrorGuidance(service string) string {
	return fmt.Sprintf(
		"This usually means %s uses a certificate signed by a private/corporate CA.\n"+
			"Set the SSL_CERT_FILE environment variable to the path of your CA bundle, e.g.:\n"+
			"  export SSL_CERT_FILE=/path/to/ca-bundle.crt",
		service,
	)
}
