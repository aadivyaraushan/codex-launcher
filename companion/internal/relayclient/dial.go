package relayclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
)

// pinnedTLSConfig builds the TLS client config the Mac uses for every
// connection to the box (both the control line and each data line). Trust
// comes from one thing only: the box leaf certificate's public key must
// equal pinnedPublicKey, byte for byte. There is deliberately no RootCAs /
// certificate-authority fallback — a box with a perfectly valid certificate
// signed by someone else is still an impostor here.
func pinnedTLSConfig(pinnedPublicKey []byte) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true, // trust is enforced by VerifyConnection below, not by a CA chain
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) != 1 {
				return errors.New("relayclient: expected exactly one peer certificate")
			}
			if !bytes.Equal(state.PeerCertificates[0].RawSubjectPublicKeyInfo, pinnedPublicKey) {
				return errors.New("relayclient: peer certificate does not match pinned box key")
			}
			return nil
		},
	}
}

// Dial opens one box-pinned TLS connection to the Mac door at addr. The
// handshake only succeeds if the box's leaf certificate public key equals
// pinnedPublicKey; any other key — even a perfectly valid, unrelated
// certificate — fails the handshake. Both the control line and every data
// line are opened this same way.
func Dial(ctx context.Context, addr string, pinnedPublicKey []byte) (*tls.Conn, error) {
	var dialer net.Dialer
	raw, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("relayclient: dial box at %s: %w", addr, err)
	}
	conn := tls.Client(raw, pinnedTLSConfig(pinnedPublicKey))
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("relayclient: pinned handshake with box at %s: %w", addr, err)
	}
	return conn, nil
}
