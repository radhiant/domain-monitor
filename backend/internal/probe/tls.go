package probe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"strings"
	"time"

	"domain-monitor/backend/internal/model"
)

// TLSProber reads the certificate presented by a host.
type TLSProber struct {
	dialer *net.Dialer
}

// NewTLSProber builds a prober with the given handshake timeout.
func NewTLSProber(timeout time.Duration) *TLSProber {
	return &TLSProber{dialer: &net.Dialer{Timeout: timeout}}
}

// Probe performs a TLS handshake and describes the leaf certificate.
//
// Verification is deliberately split from the handshake: dialling with
// InsecureSkipVerify gets us the chain even when it is expired or misissued,
// which is precisely the case the dashboard needs to display. The chain is
// then verified separately so the failure is reported rather than hidden.
func (p *TLSProber) Probe(ctx context.Context, target model.Target) *model.CertInfo {
	info := &model.CertInfo{CheckedAt: time.Now().UTC()}

	if strings.HasPrefix(target.URL, "http://") {
		// Plain HTTP by choice; there is no certificate to report.
		return nil
	}

	// A target may carry an explicit port ("api.example.com:8443"); if it does
	// not, TLS lives on 443.
	host, addr := target.Host, target.Host
	if h, _, err := net.SplitHostPort(target.Host); err == nil {
		host = h
	} else {
		addr = net.JoinHostPort(target.Host, "443")
	}

	rawConn, err := p.dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		info.Error = cleanError(err)
		return info
	}

	conn := tls.Client(rawConn, &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: true,
	})
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = rawConn.SetDeadline(deadline)
	}

	if err := conn.HandshakeContext(ctx); err != nil {
		info.Error = cleanError(err)
		return info
	}

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		info.Error = "server presented no certificate"
		return info
	}

	leaf := state.PeerCertificates[0]
	notBefore := leaf.NotBefore.UTC()
	notAfter := leaf.NotAfter.UTC()
	days := daysUntil(notAfter)

	info.NotBefore = &notBefore
	info.NotAfter = &notAfter
	info.DaysLeft = &days
	info.Issuer = issuerName(leaf)
	info.Subject = leaf.Subject.CommonName
	info.SANs = leaf.DNSNames
	info.TLSVersion = tlsVersionName(state.Version)
	info.Cipher = tls.CipherSuiteName(state.CipherSuite)

	if err := verifyChain(host, state.PeerCertificates); err != nil {
		info.Error = cleanError(err)
	}

	return info
}

// verifyChain runs the standard verification the dial skipped.
func verifyChain(host string, chain []*x509.Certificate) error {
	intermediates := x509.NewCertPool()
	for _, cert := range chain[1:] {
		intermediates.AddCert(cert)
	}

	_, err := chain[0].Verify(x509.VerifyOptions{
		DNSName:       host,
		Intermediates: intermediates,
	})
	return err
}

// issuerName prefers the CA's organisation, which is what operators recognise
// ("Let's Encrypt"), and falls back to the common name.
func issuerName(cert *x509.Certificate) string {
	if len(cert.Issuer.Organization) > 0 && cert.Issuer.Organization[0] != "" {
		return cert.Issuer.Organization[0]
	}
	return cert.Issuer.CommonName
}

func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS13:
		return "TLS 1.3"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS10:
		return "TLS 1.0"
	default:
		return "unknown"
	}
}

// daysUntil rounds down, so "1 day left" never displays for something that
// expires in the next few minutes.
func daysUntil(t time.Time) int {
	return int(time.Until(t).Hours() / 24)
}
