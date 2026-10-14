package feed

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

const maxFeedBytes int64 = 5 << 20

var ErrBlockedFeedAddress = errors.New("feed destination is blocked")

var blockedFeedRanges = [...]netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
}

var feedIPv6Range = netip.MustParsePrefix("2000::/3")

func ValidateFeedURL(u *url.URL) error {
	if u == nil {
		return errors.New("feed URL is required")
	}
	
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("feed URL must use http or https scheme")
	}
	
	hostname := u.Hostname()
	if hostname == "" || u.Opaque != "" {
		return errors.New("feed URL must have a valid hostname")
	}
	
	if u.User != nil {
		return errors.New("feed URL must not contain user info")
	}
	
	if strings.Contains(hostname, "%") {
		return errors.New("feed URL must not contain an IPv6 zone")
	}
	
	port := u.Port()
	if port != "" {
		expectedPort := "80"
		if u.Scheme == "https" {
			expectedPort = "443"
		}
		if port != expectedPort {
			return errors.New("feed URL must not contain a non-default port")
		}
	}
	
	if ip, err := netip.ParseAddr(hostname); err == nil && !isAllowedFeedIP(ip) {
		return ErrBlockedFeedAddress
	}
	
	return nil
}

func isAllowedFeedIP(ip netip.Addr) bool {
	if !ip.IsValid() || ip.Zone() != "" {
		return false
	}
	
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	
	if ip.Is6() && !feedIPv6Range.Contains(ip) {
		return false
	}
	
	for _, prefix := range blockedFeedRanges {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

type publicFeedDialer struct {
	lookup func(context.Context, string, string) ([]netip.Addr, error)
	dial   func(context.Context, string, string) (net.Conn, error)
}

func (d publicFeedDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, errors.New("feed connections must use TCP")
	}
	
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid feed destination: %w", err)
	}

	if port != "80" && port != "443" {
		return nil, ErrBlockedFeedAddress
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	ips, err := d.lookup(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve feed hostname: %w", err)
	}
	
	if len(ips) == 0 {
		return nil, errors.New("feed hostname has no addresses")
	}

	for _, ip := range ips {
		if !isAllowedFeedIP(ip) {
			return nil, ErrBlockedFeedAddress
		}
	}
	
	var failures []error
	for _, ip := range ips {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		target := net.JoinHostPort(ip.Unmap().String(), port)
		conn, err := d.dial(ctx, network, target)
		if err == nil {
			return conn, nil
		}
		failures = append(failures, err)
	}
	
	return nil, fmt.Errorf("connect to feed: %w", errors.Join(failures...))
}

type validatedFeedTransport struct{ transport *http.Transport }

func (t *validatedFeedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := ValidateFeedURL(req.URL); err != nil {
		return nil, err
	}
	return t.transport.RoundTrip(req)
}

func (t *validatedFeedTransport) CloseIdleConnections() { t.transport.CloseIdleConnections() }

func newFeedHTTPClient(dialer publicFeedDialer) *http.Client {
	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            dialer.DialContext,
		ForceAttemptHTTP2:      true,
		TLSHandshakeTimeout:    3 * time.Second,
		ResponseHeaderTimeout:  5 * time.Second,
		MaxResponseHeaderBytes: 64 << 10,
		IdleConnTimeout:        90 * time.Second,
		MaxIdleConns:           32,
		MaxIdleConnsPerHost:    2,
		MaxConnsPerHost:        4,
	}
	return &http.Client{
		Transport: &validatedFeedTransport{transport: transport},
		Timeout:   8 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many feed redirects")
			}
			if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
				return errors.New("feed redirect must not downgrade HTTPS")
			}
			return ValidateFeedURL(req.URL)
		},
	}
}

var defaultFeedHTTPClient = newFeedHTTPClient(publicFeedDialer{
	lookup: net.DefaultResolver.LookupNetIP,
	dial:   (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
})

func fetchSourceFeed(ctx context.Context, client *http.Client, parser *gofeed.Parser, feedURL string) (*gofeed.Feed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", parser.UserAgent)
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, gofeed.HTTPError{StatusCode: response.StatusCode, Status: response.Status}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxFeedBytes+1))
	if err != nil {
		return nil, err
	}

	if int64(len(body)) > maxFeedBytes {
		return nil, gofeed.ErrResponseTooLarge
	}

	return parser.Parse(bytes.NewReader(body))
}
