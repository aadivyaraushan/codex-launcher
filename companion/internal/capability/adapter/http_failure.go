package adapter

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
)

// ClassifyHTTPFailure decides what an adapter should tell the rest of the
// system after an HTTP round-trip returns err.
//
// The rule: a failed read (GET or HEAD) is never ambiguous, because reading
// changes nothing — err is returned as-is. For anything else, the question
// is whether the request ever left this machine. If it demonstrably did
// not — the hostname would not resolve, the connection was refused, there
// was no route to the host, the network was unreachable, or the TLS
// certificate was rejected — then nothing could have happened on the other
// end, so err is returned as-is too. Everything else is treated as an
// unknown outcome: the request may have reached the other side and only the
// reply got lost, so the caller cannot promise it did not happen.
//
// An error type this function does not recognise also comes back as an
// unknown outcome. By the time http.Client.Do fails with something nobody
// has enumerated, a connection has usually already been made, so treating
// the unrecognised case as "we don't know" is the safer of the two
// possible mistakes: a needless "go check" is annoying, but telling
// someone a message never sent when it did can send a duplicate to a real
// person, which cannot be undone.
func ClassifyHTTPFailure(adapterID, method string, err error) error {
	if err == nil {
		return nil
	}
	if method == http.MethodGet || method == http.MethodHead {
		return err
	}
	if requestNeverLeftTheMachine(err) {
		return err
	}
	return &OutcomeUnknownError{AdapterID: adapterID, Verb: method, Cause: err}
}

// requestNeverLeftTheMachine reports whether err demonstrably happened
// before any bytes reached the network: a name that would not resolve, a
// dial that never connected, or a TLS certificate that was rejected during
// the handshake.
func requestNeverLeftTheMachine(err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return true
	}
	// The TLS handshake finishes before the request is written, so every way
	// it can be refused belongs here — not just the untrusted-authority one.
	var unknownAuthority x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthority) {
		return true
	}
	var wrongHost x509.HostnameError
	if errors.As(err, &wrongHost) {
		return true
	}
	var invalidCert x509.CertificateInvalidError
	if errors.As(err, &invalidCert) {
		return true
	}
	var verifyFailed *tls.CertificateVerificationError
	if errors.As(err, &verifyFailed) {
		return true
	}
	return false
}
