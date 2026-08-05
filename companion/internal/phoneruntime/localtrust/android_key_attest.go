package localtrust

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	_ "embed"
)

//go:embed roots/google_attestation_roots.pem
var googleAttestationRootsPEM []byte

var (
	ErrAttestChain   = errors.New("attestation: certificate chain invalid")
	ErrAttestParse   = errors.New("attestation: key description parse failed")
	oidAndroidAttest = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 11129, 2, 1, 17}
)

type ExpectedOperator struct {
	PackageName       string
	SigningCertSHA256 string
}

type ParsedAndroidAttestation struct {
	Challenge        []byte
	Level            SecurityLevel
	PackageNames     []string
	SignatureDigests []string
	RootFingerprint  string
	LeafSerialHex    string
}

func LoadGoogleAttestationRoots() (*x509.CertPool, map[string]struct{}, error) {
	pool := x509.NewCertPool()
	fps := map[string]struct{}{}
	rest := googleAttestationRootsPEM
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, nil, err
		}
		pool.AddCert(cert)
		sum := sha256.Sum256(cert.Raw)
		fps[strings.ToLower(hex.EncodeToString(sum[:]))] = struct{}{}
	}
	if len(fps) == 0 {
		return nil, nil, fmt.Errorf("%w: no roots embedded", ErrAttestChain)
	}
	return pool, fps, nil
}

func VerifyAndroidKeyAttestationChain(derChain [][]byte, roots *x509.CertPool) (*x509.Certificate, string, error) {
	if len(derChain) == 0 {
		return nil, "", fmt.Errorf("%w: empty chain", ErrAttestChain)
	}
	certs := make([]*x509.Certificate, 0, len(derChain))
	for i, der := range derChain {
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, "", fmt.Errorf("%w: cert[%d]: %v", ErrAttestChain, i, err)
		}
		certs = append(certs, cert)
	}
	leaf := certs[0]
	intermediates := x509.NewCertPool()
	for _, cert := range certs[1:] {
		intermediates.AddCert(cert)
	}
	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}
	chains, err := leaf.Verify(opts)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrAttestChain, err)
	}
	if len(chains) == 0 || len(chains[0]) == 0 {
		return nil, "", fmt.Errorf("%w: no verified path", ErrAttestChain)
	}
	root := chains[0][len(chains[0])-1]
	sum := sha256.Sum256(root.Raw)
	return leaf, strings.ToLower(hex.EncodeToString(sum[:])), nil
}

func ParseAndroidKeyAttestation(leaf *x509.Certificate) (ParsedAndroidAttestation, error) {
	var out ParsedAndroidAttestation
	var extBytes []byte
	for _, ext := range leaf.Extensions {
		if ext.Id.Equal(oidAndroidAttest) {
			extBytes = ext.Value
			break
		}
	}
	if len(extBytes) == 0 {
		return out, fmt.Errorf("%w: missing android key attestation extension", ErrAttestParse)
	}
	var keyDescBytes []byte
	var raw asn1.RawValue
	if _, err := asn1.Unmarshal(extBytes, &raw); err == nil && raw.Tag == asn1.TagOctetString {
		keyDescBytes = raw.Bytes
	} else {
		keyDescBytes = extBytes
	}
	var desc struct {
		AttestationVersion       int
		AttestationSecurityLevel asn1.Enumerated
		KeymasterVersion         int
		KeymasterSecurityLevel   asn1.Enumerated
		AttestationChallenge     []byte
		UniqueID                 []byte
		SoftwareEnforced         asn1.RawValue
		TeeEnforced              asn1.RawValue
	}
	if _, err := asn1.Unmarshal(keyDescBytes, &desc); err != nil {
		return out, fmt.Errorf("%w: keyDescription: %v", ErrAttestParse, err)
	}
	out.Challenge = desc.AttestationChallenge
	out.Level = securityLevelFromEnum(desc.AttestationSecurityLevel)
	packages, digests, err := parseAttestationApplicationID(desc.TeeEnforced.FullBytes)
	if err != nil || len(packages) == 0 {
		packages2, digests2, err2 := parseAttestationApplicationID(desc.SoftwareEnforced.FullBytes)
		if err2 == nil && len(packages2) > 0 {
			packages, digests = packages2, digests2
			if out.Level != SecurityTrustedEnvironment && out.Level != SecurityStrongBox {
				out.Level = SecuritySoftware
			}
		} else if err != nil {
			return out, err
		}
	}
	out.PackageNames = packages
	out.SignatureDigests = digests
	out.LeafSerialHex = strings.ToLower(leaf.SerialNumber.Text(16))
	return out, nil
}

func securityLevelFromEnum(v asn1.Enumerated) SecurityLevel {
	switch int(v) {
	case 1:
		return SecurityTrustedEnvironment
	case 2:
		return SecurityStrongBox
	default:
		return SecuritySoftware
	}
}

func parseAttestationApplicationID(authListDER []byte) ([]string, []string, error) {
	if len(authListDER) == 0 {
		return nil, nil, fmt.Errorf("%w: empty auth list", ErrAttestParse)
	}
	appID, ok := findContextTagBytes(authListDER, 709)
	if !ok {
		return nil, nil, fmt.Errorf("%w: attestationApplicationId missing", ErrAttestParse)
	}
	var inner asn1.RawValue
	if _, err := asn1.Unmarshal(appID, &inner); err == nil && inner.Tag == asn1.TagOctetString {
		appID = inner.Bytes
	}
	var seq asn1.RawValue
	if _, err := asn1.Unmarshal(appID, &seq); err != nil {
		return nil, nil, fmt.Errorf("%w: app id seq: %v", ErrAttestParse, err)
	}
	if seq.Tag != asn1.TagSequence {
		return nil, nil, fmt.Errorf("%w: app id not sequence", ErrAttestParse)
	}
	rest := seq.Bytes
	var packageInfos asn1.RawValue
	var err error
	rest, err = asn1.Unmarshal(rest, &packageInfos)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: packageInfos: %v", ErrAttestParse, err)
	}
	var digestsSet asn1.RawValue
	if _, err := asn1.Unmarshal(rest, &digestsSet); err != nil {
		return nil, nil, fmt.Errorf("%w: signatureDigests: %v", ErrAttestParse, err)
	}
	packages := []string{}
	piRest := packageInfos.Bytes
	for len(piRest) > 0 {
		var pkgSeq asn1.RawValue
		piRest, err = asn1.Unmarshal(piRest, &pkgSeq)
		if err != nil {
			break
		}
		var pkg struct {
			Name    []byte
			Version int
		}
		if _, err := asn1.Unmarshal(pkgSeq.FullBytes, &pkg); err != nil {
			if _, err2 := asn1.Unmarshal(pkgSeq.Bytes, &pkg); err2 != nil {
				continue
			}
		}
		if len(pkg.Name) > 0 {
			packages = append(packages, string(pkg.Name))
		}
	}
	digests := []string{}
	dRest := digestsSet.Bytes
	for len(dRest) > 0 {
		var dig asn1.RawValue
		dRest, err = asn1.Unmarshal(dRest, &dig)
		if err != nil {
			break
		}
		digests = append(digests, strings.ToLower(hex.EncodeToString(dig.Bytes)))
	}
	return packages, digests, nil
}

func findContextTagBytes(der []byte, tag int) ([]byte, bool) {
	return findContextTagBytesDepth(der, tag, 0)
}

func findContextTagBytesDepth(der []byte, tag int, depth int) ([]byte, bool) {
	if depth > 32 || len(der) == 0 {
		return nil, false
	}
	rest := der
	var outer asn1.RawValue
	if _, err := asn1.Unmarshal(der, &outer); err == nil && outer.Tag == asn1.TagSequence && outer.Class == asn1.ClassUniversal {
		rest = outer.Bytes
	}
	for len(rest) > 0 {
		var rv asn1.RawValue
		next, err := asn1.Unmarshal(rest, &rv)
		if err != nil {
			return nil, false
		}
		rest = next
		if rv.Class == asn1.ClassContextSpecific && rv.Tag == tag {
			return rv.Bytes, true
		}
		if rv.IsCompound && len(rv.Bytes) > 0 {
			if b, ok := findContextTagBytesDepth(rv.Bytes, tag, depth+1); ok {
				return b, true
			}
		}
	}
	return nil, false
}
