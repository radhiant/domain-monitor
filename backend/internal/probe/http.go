// Package probe contains the four outside-in checks the agent runs against
// every target: reachability, TLS certificate, DNS resolution and domain
// registration.
package probe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"os"
	"strings"
	"time"

	"domain-monitor/backend/internal/model"
)

const maxRedirects = 5

// bodyReadCap bounds how much of a response we pull. We need to read the body
// for the timing to mean anything, but a large page would distort the latency
// figure and waste bandwidth on every single check.
const bodyReadCap = 64 * 1024

// HTTPProber performs reachability checks.
type HTTPProber struct {
	client    *http.Client
	userAgent string
}

// NewHTTPProber builds a prober with its own client.
//
// Keep-alives are disabled deliberately: a reused connection would skip the
// DNS lookup and TCP handshake, which are exactly the failures this check
// exists to catch.
func NewHTTPProber(timeout time.Duration, userAgent string) *HTTPProber {
	transport := &http.Transport{
		DisableKeepAlives:     true,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		DialContext: (&net.Dialer{
			Timeout: timeout,
		}).DialContext,
	}

	return &HTTPProber{
		client: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
		userAgent: userAgent,
	}
}

// Probe issues one GET and reports what happened.
func (p *HTTPProber) Probe(ctx context.Context, target model.Target) *model.HTTPResult {
	res := &model.HTTPResult{CheckedAt: time.Now().UTC()}

	var redirects []string
	var ttfb time.Duration
	start := time.Now()

	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() {
			ttfb = time.Since(start)
		},
	}

	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodGet, target.URL, nil)
	if err != nil {
		res.Error = err.Error()
		res.ErrorKind = model.ErrHTTP
		return res
	}
	req.Header.Set("User-Agent", p.userAgent)
	req.Header.Set("Accept", "*/*")
	// Compressed responses would make the byte count meaningless and cost CPU
	// on both ends for a payload that is immediately discarded.
	req.Header.Set("Accept-Encoding", "identity")

	client := *p.client
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}
		redirects = append(redirects, r.URL.String())
		return nil
	}

	resp, err := client.Do(req)
	if err != nil {
		res.LatencyMS = msSince(start)
		res.Error = cleanError(err)
		res.ErrorKind = classifyError(err)
		res.Redirects = redirects
		return res
	}
	defer resp.Body.Close()

	n, _ := io.Copy(io.Discard, io.LimitReader(resp.Body, bodyReadCap))

	res.LatencyMS = msSince(start)
	res.TTFBMS = float64(ttfb.Microseconds()) / 1000
	res.StatusCode = resp.StatusCode
	res.BodyBytes = n
	res.FinalURL = resp.Request.URL.String()
	res.Redirects = redirects
	res.Up = resp.StatusCode < 500

	if !res.Up {
		res.ErrorKind = model.ErrHTTP
		res.Error = resp.Status
	}

	return res
}

// classifyError maps a transport error onto the category shown on the wall, so
// an operator sees "DNS" or "TLS" rather than a wrapped Go error chain.
func classifyError(err error) model.ErrorKind {
	if err == nil {
		return model.ErrNone
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return model.ErrDNS
	}

	var certErr *x509.CertificateInvalidError
	var hostErr x509.HostnameError
	var authErr x509.UnknownAuthorityError
	var recordErr tls.RecordHeaderError
	if errors.As(err, &certErr) || errors.As(err, &hostErr) || errors.As(err, &authErr) || errors.As(err, &recordErr) {
		return model.ErrTLS
	}

	// Timeout is checked after the specific kinds above: a DNS lookup that
	// times out is more usefully reported as a DNS failure.
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return model.ErrTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return model.ErrTimeout
	}

	if strings.Contains(err.Error(), "tls:") || strings.Contains(err.Error(), "certificate") {
		return model.ErrTLS
	}

	return model.ErrConnect
}

// cleanError strips the Go request prefix that repeats the URL already shown
// next to the error on screen.
func cleanError(err error) string {
	msg := err.Error()
	if i := strings.Index(msg, ": "); i >= 0 && strings.HasPrefix(msg, "Get \"") {
		msg = msg[i+2:]
	}
	return strings.TrimSpace(msg)
}

func msSince(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000
}
