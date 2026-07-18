// Package relaybox implements the "meeting-point box" described in
// planning/relay-box-build-plan.md: a small program that lets a phone reach
// a Mac companion without both sides being on the same private network.
//
// The box has two doors, guarded differently on purpose:
//   - the phone door is a plain raw TCP port. The box never terminates TLS
//     here, so the phone's sealed connection to the Mac passes through
//     untouched.
//   - the Mac door is TLS, using the box's own key (this file generates it).
//     The Mac proves who it is with a registration secret sent over that
//     encrypted line; the box proves who it is with this certificate, which
//     the Mac pins by public key (see the relayclient package).
//
// The box never imports the phone/Mac identity or pairing packages: it does
// not need their keys to do its job, and holding none of them is what makes
// "the box can't read your messages" true by construction, not by promise.
package relaybox

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"time"
)

// GenerateSelfSignedCertificate creates the box's own TLS identity for the
// Mac door: a fresh ECDSA key and a self-signed certificate for it. The Mac
// pins the resulting certificate's public key (relayclient.Dial) instead of
// trusting a certificate authority, so nothing here needs to be signed by
// anyone else.
func GenerateSelfSignedCertificate(random io.Reader, now time.Time) (tls.Certificate, error) {
	if random == nil {
		random = rand.Reader
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), random)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("relaybox: generate box key: %w", err)
	}
	serialBytes := make([]byte, 16)
	if _, err := io.ReadFull(random, serialBytes); err != nil {
		return tls.Certificate{}, fmt.Errorf("relaybox: generate box certificate serial: %w", err)
	}
	serial := new(big.Int).SetBytes(serialBytes)
	if serial.Sign() == 0 {
		serial.SetInt64(1)
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "Codex Launcher Relay Box"},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(random, template, template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("relaybox: create box certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("relaybox: parse box certificate: %w", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, nil
}
