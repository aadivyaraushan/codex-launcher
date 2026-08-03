package adapter

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"syscall"
	"testing"
)

// Telling someone "we don't know" when we do know.
//
// Seven adapters (slack client.go:181, todoist :141, outlook :178, msteams
// :143, gcalendar :192, gdrive :172, notion session.go:106) all decide the same
// thing the same way after their HTTP round-trip fails:
//
//	if method != http.MethodGet {
//	    return &adapter.OutcomeUnknownError{...}
//	}
//
// The GET half of that is right — a read changes nothing, so a failed read is
// never ambiguous. The other half is too broad. `http.Client.Do` also returns an
// error when the request never left the machine at all: the hostname did not
// resolve, the connection was refused, the TLS handshake failed. Nothing was
// sent in any of those. Calling them "we don't know" makes the phone block the
// next prompt and send the user off to check an app where nothing happened.
//
// The same rule copied into seven files also drifts. One function, seven
// callers.
//
// Where the line sits: an error is only ambiguous once the connection is up and
// the request is on the wire. Everything that fails before that point is a
// plain, certain failure. Anything after it — a timeout, a deadline, a
// connection that dies mid-reply — is ambiguous, because the server may have
// acted before we stopped listening. That fallback is deliberate and the harm
// is lopsided: a needless "go and check" is annoying, while a duplicate message
// to a real person cannot be taken back.

// timeoutError is what net/http hands back on a client timeout: an error whose
// Timeout() reports true, rather than context.DeadlineExceeded itself.
type timeoutError struct{}

func (timeoutError) Error() string   { return "net/http: request canceled (Client.Timeout exceeded)" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func postFailure(cause error) error {
	return fmt.Errorf("slack: POST request failed: %w", &url.Error{Op: "Post", URL: "https://slack.com/api/chat.postMessage", Err: cause})
}

func isUnknown(err error) bool {
	var unknown *OutcomeUnknownError
	return errors.As(err, &unknown)
}

// Nothing left the machine: these must stay plain failures.
func TestARequestThatNeverLeftTheMachineIsAPlainFailure(t *testing.T) {
	cases := []struct {
		name  string
		cause error
	}{
		{"the hostname did not resolve", &net.DNSError{Err: "no such host", Name: "slack.com", IsNotFound: true}},
		{"the connection was refused", &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}},
		{"there was no route to the host", &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.EHOSTUNREACH)}},
		{"the network was unreachable", &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ENETUNREACH)}},
		{"the certificate was not trusted", x509.UnknownAuthorityError{}},
		// A TLS handshake happens before the request is written, so every way
		// it can be refused belongs on this side of the line — not just the
		// untrusted-authority one.
		{"the certificate was for the wrong host", x509.HostnameError{Host: "slack.com", Certificate: &x509.Certificate{}}},
		{"the certificate had expired", x509.CertificateInvalidError{Reason: x509.Expired}},
		{"the handshake rejected the certificate", &tls.CertificateVerificationError{Err: errors.New("bad chain")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClassifyHTTPFailure("slack", http.MethodPost, postFailure(c.cause))

			if isUnknown(got) {
				t.Fatalf("a request that never went out was reported as an unknown outcome: %v", got)
			}
			if !errors.Is(got, c.cause) {
				t.Fatalf("the original reason stopped being reachable: %v", got)
			}
		})
	}
}

// The request was on the wire when things went wrong: these are the real
// ambiguous ones, and the whole reason the marker exists.
func TestARequestThatWasAlreadySentIsAnUnknownOutcome(t *testing.T) {
	cases := []struct {
		name  string
		cause error
	}{
		{"the deadline passed while waiting", context.DeadlineExceeded},
		{"the caller gave up", context.Canceled},
		{"the client timed out", timeoutError{}},
		{"the connection closed before the reply", io.EOF},
		{"the reply was cut off part-way", io.ErrUnexpectedEOF},
		{"the peer reset the connection", &net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClassifyHTTPFailure("slack", http.MethodPost, postFailure(c.cause))

			var unknown *OutcomeUnknownError
			if !errors.As(got, &unknown) {
				t.Fatalf("a send that may already have landed was reported as a definite failure: %v", got)
			}
			if unknown.AdapterID != "slack" || unknown.Verb != http.MethodPost {
				t.Fatalf("the marker lost what it was about: %+v", unknown)
			}
			if !errors.Is(got, c.cause) {
				t.Fatalf("the original reason stopped being reachable: %v", got)
			}
		})
	}
}

// An error nobody enumerated. By the time Do fails with something unrecognised
// the connection is normally already up, so this leans to "we don't know" —
// the safe side, because the other side sends someone a second copy of their
// own message.
func TestAnUnrecognisedTransportErrorLeansToUnknown(t *testing.T) {
	got := ClassifyHTTPFailure("todoist", http.MethodPost, postFailure(errors.New("something nobody has seen before")))

	if !isUnknown(got) {
		t.Fatalf("an unrecognised mid-flight error should not be claimed as a definite failure: %v", got)
	}
}

// A read changes nothing, so a failed read is never ambiguous — whatever went
// wrong, the world is exactly as it was.
func TestAFailedReadIsNeverAmbiguous(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			got := ClassifyHTTPFailure("gdrive", method, postFailure(context.DeadlineExceeded))

			if isUnknown(got) {
				t.Fatalf("a failed %s was treated as an unknown outcome: %v", method, got)
			}
		})
	}
}

// Handing back nil for a nil error keeps the call site a one-liner: adapters
// can pass whatever Do gave them without an if around it.
func TestNoErrorClassifiesToNoError(t *testing.T) {
	if got := ClassifyHTTPFailure("slack", http.MethodPost, nil); got != nil {
		t.Fatalf("classifying a success produced an error: %v", got)
	}
}

// A timeout that is wrapped several layers deep must still be recognised —
// adapters add their own context on the way up.
func TestTheDecisionSurvivesWrapping(t *testing.T) {
	deep := fmt.Errorf("notion: creating page: %w", fmt.Errorf("http: %w", &url.Error{Op: "Post", URL: "https://api.notion.com", Err: context.DeadlineExceeded}))

	if !isUnknown(ClassifyHTTPFailure("notion", http.MethodPost, deep)) {
		t.Fatal("a wrapped timeout was no longer recognised as ambiguous")
	}
}

var _ net.Error = timeoutError{}
