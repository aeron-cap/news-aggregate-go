package feed

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/database"
	"github.com/mmcdole/gofeed"
)

func TestValidateFeedURL(t *testing.T) {
	for _, tc := range []struct {
		url     string
		allowed bool
	}{
		{"https://example.com/feed", true},
		{"http://example.com:80/feed", true},
		{"https://example.com:443/feed", true},
		{"https://1.1.1.1/feed", true},
		{"https://[2606:4700:4700::1111]/feed", true},
		{"/feed", false},
		{"https:feed", false},
		{"ftp://example.com/feed", false},
		{"https://user:password@example.com/feed", false},
		{"https://example.com:8080/feed", false},
		{"https://example.com:80/feed", false},
		{"http://127.0.0.1/feed", false},
		{"http://169.254.169.254/feed", false},
		{"http://[::ffff:127.0.0.1]/feed", false},
		{"http://[fe80::1%25eth0]/feed", false},
	} {
		t.Run(tc.url, func(t *testing.T) {
			u, err := url.Parse(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateFeedURL(u); (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v err=%v", tc.allowed, err)
			}
		})
	}
	if err := ValidateFeedURL(nil); err == nil {
		t.Fatal("nil URL accepted")
	}
}

func TestAllowedFeedIPPolicy(t *testing.T) {
	for _, tc := range []struct {
		ip      string
		allowed bool
	}{
		{"1.1.1.1", true}, {"8.8.8.8", true}, {"100.63.255.255", true},
		{"100.128.0.1", true}, {"192.0.1.1", true},
		{"2606:4700:4700::1111", true}, {"::ffff:1.1.1.1", true},
		{"0.0.0.0", false}, {"0.1.2.3", false}, {"127.0.0.1", false},
		{"10.0.0.1", false}, {"172.16.0.1", false}, {"192.168.1.1", false},
		{"169.254.169.254", false}, {"100.64.0.1", false}, {"100.127.255.255", false},
		{"192.0.0.9", false}, {"192.0.2.1", false}, {"192.88.99.2", false},
		{"198.18.0.1", false}, {"198.51.100.1", false}, {"203.0.113.1", false},
		{"224.0.0.1", false}, {"240.0.0.1", false}, {"255.255.255.255", false},
		{"::", false}, {"::1", false}, {"::ffff:127.0.0.1", false},
		{"fd00::1", false}, {"fe80::1", false}, {"ff02::1", false},
		{"64:ff9b::a00:1", false}, {"100::1", false}, {"2001::1", false},
		{"2001:db8::1", false}, {"2002:7f00:1::1", false}, {"3fff::1", false},
		{"5f00::1", false}, {"2606:4700:4700::1111%eth0", false},
	} {
		t.Run(tc.ip, func(t *testing.T) {
			if got := isAllowedFeedIP(netip.MustParseAddr(tc.ip)); got != tc.allowed {
				t.Fatalf("allowed=%v want=%v", got, tc.allowed)
			}
		})
	}
	if isAllowedFeedIP(netip.Addr{}) {
		t.Fatal("invalid IP accepted")
	}
}

func TestPublicFeedDialerRejectsDNSBeforeConnecting(t *testing.T) {
	dnsFailure := errors.New("DNS unavailable")
	for _, tc := range []struct {
		name string
		ips  []netip.Addr
		err  error
	}{
		{"private", []netip.Addr{netip.MustParseAddr("10.0.0.1")}, nil},
		{"mixed", []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("127.0.0.1")}, nil},
		{"mapped", []netip.Addr{netip.MustParseAddr("::ffff:169.254.169.254")}, nil},
		{"IPv6", []netip.Addr{netip.MustParseAddr("fd00::1")}, nil},
		{"invalid", []netip.Addr{{}}, nil},
		{"empty", nil, nil},
		{"failure", []netip.Addr{netip.MustParseAddr("1.1.1.1")}, dnsFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dials := 0
			dialer := publicFeedDialer{
				lookup: func(context.Context, string, string) ([]netip.Addr, error) { return tc.ips, tc.err },
				dial: func(context.Context, string, string) (net.Conn, error) {
					dials++
					return nil, errors.New("unexpected dial")
				},
			}
			if _, err := dialer.DialContext(context.Background(), "tcp", "feed.example:80"); err == nil {
				t.Fatal("unsafe or failed DNS accepted")
			}
			if dials != 0 {
				t.Fatalf("dialed %d times before rejecting DNS", dials)
			}
		})
	}
}

func TestPublicFeedDialerPinsValidatedIPsAndRetries(t *testing.T) {
	lookups := 0
	var targets []string
	conn, peer := net.Pipe()
	defer conn.Close()
	defer peer.Close()
	dialer := publicFeedDialer{
		lookup: func(_ context.Context, network, host string) ([]netip.Addr, error) {
			lookups++
			if network != "ip" || host != "feed.example" {
				t.Fatalf("lookup %s %s", network, host)
			}
			if lookups > 1 {
				return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
			}
			return []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("::ffff:8.8.8.8")}, nil
		},
		dial: func(_ context.Context, _ string, target string) (net.Conn, error) {
			targets = append(targets, target)
			if len(targets) == 1 {
				return nil, errors.New("first address unavailable")
			}
			return conn, nil
		},
	}
	if got, err := dialer.DialContext(context.Background(), "tcp", "feed.example:443"); err != nil || got != conn {
		t.Fatalf("connection=%v err=%v", got, err)
	}
	if lookups != 1 || strings.Join(targets, ",") != "1.1.1.1:443,8.8.8.8:443" {
		t.Fatalf("lookup count=%d targets=%v", lookups, targets)
	}
}

func TestPublicFeedDialerCancellationAndDestinationRules(t *testing.T) {
	dialer := publicFeedDialer{
		lookup: func(ctx context.Context, _, _ string) ([]netip.Addr, error) { <-ctx.Done(); return nil, ctx.Err() },
		dial:   func(context.Context, string, string) (net.Conn, error) { t.Fatal("unexpected dial"); return nil, nil },
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := dialer.DialContext(ctx, "tcp", "feed.example:80"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation err=%v", err)
	}
	for _, tc := range []struct{ network, address string }{{"udp", "feed.example:80"}, {"tcp", "feed.example:8080"}, {"tcp", "invalid"}} {
		if _, err := dialer.DialContext(context.Background(), tc.network, tc.address); err == nil {
			t.Fatalf("accepted %s %s", tc.network, tc.address)
		}
	}
}

func fixtureFeedClient(t *testing.T, address string) *http.Client {
	t.Helper()
	client := newFeedHTTPClient(publicFeedDialer{
		lookup: func(_ context.Context, _, host string) ([]netip.Addr, error) {
			if host == "private.example" {
				return []netip.Addr{netip.MustParseAddr("10.0.0.1")}, nil
			}
			return []netip.Addr{netip.MustParseAddr("1.1.1.1")}, nil
		},
		dial: func(ctx context.Context, network, target string) (net.Conn, error) {
			if target != "1.1.1.1:80" && target != "1.1.1.1:443" {
				return nil, fmt.Errorf("unexpected destination %s", target)
			}
			return (&net.Dialer{}).DialContext(ctx, network, address)
		},
	})
	t.Cleanup(client.CloseIdleConnections)
	return client
}

func TestFeedClientValidatesRequestsRedirectsAndIgnoresProxy(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Host != "feed.example" {
			t.Errorf("original HTTP host lost: %s", r.Host)
		}
		switch r.URL.Path {
		case "/private":
			http.Redirect(w, r, "http://private.example/feed", http.StatusFound)
		case "/metadata":
			http.Redirect(w, r, "http://169.254.169.254/feed", http.StatusFound)
		case "/credentials":
			http.Redirect(w, r, "http://user:secret@feed.example/feed", http.StatusFound)
		case "/port":
			http.Redirect(w, r, "http://feed.example:8080/feed", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/redirect":
			http.Redirect(w, r, "/feed", http.StatusFound)
		default:
			fmt.Fprint(w, "valid public response")
		}
	}))
	defer server.Close()
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	client := fixtureFeedClient(t, server.Listener.Addr().String())
	for _, path := range []string{"/feed", "/redirect"} {
		response, err := client.Get("http://feed.example" + path)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || string(body) != "valid public response" {
			t.Fatalf("body=%q err=%v", body, err)
		}
	}
	for _, path := range []string{"/private", "/metadata", "/credentials", "/port", "/loop"} {
		before := hits.Load()
		if response, err := client.Get("http://feed.example" + path); err == nil {
			response.Body.Close()
			t.Fatalf("unsafe redirect or loop accepted: %s", path)
		}
		wantHits := int64(1)
		if path == "/loop" {
			wantHits = 3
		}
		if delta := hits.Load() - before; delta != wantHits {
			t.Fatalf("path=%s hits=%d want=%d", path, delta, wantHits)
		}
	}
	before := hits.Load()
	for _, rawURL := range []string{"http://127.0.0.1/feed", "http://[::ffff:127.0.0.1]/feed", "http://private.example/feed", "http://user:secret@feed.example/feed", "http://feed.example:8080/feed", "ftp://feed.example/feed"} {
		if response, err := client.Get(rawURL); err == nil {
			response.Body.Close()
			t.Fatalf("unsafe initial URL accepted: %s", rawURL)
		}
	}
	if hits.Load() != before {
		t.Fatal("unsafe initial URL reached fixture")
	}
}

func TestFeedParserBoundsResponsesAndClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slow":
			<-r.Context().Done()
		case "/large":
			fmt.Fprintf(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>Feed</title><description>%s</description></channel></rss>`, strings.Repeat("x", int(maxFeedBytes)+1))
		case "/compressed":
			w.Header().Set("Content-Encoding", "gzip")
			compressed := gzip.NewWriter(w)
			fmt.Fprintf(compressed, `<?xml version="1.0"?><rss version="2.0"><channel><title>Feed</title><description>%s</description></channel></rss>`, strings.Repeat("x", int(maxFeedBytes)+1))
			compressed.Close()
		case "/headers":
			w.Header().Set("X-Large", strings.Repeat("x", 65<<10))
		case "/missing":
			w.WriteHeader(http.StatusNotFound)
		default:
			fmt.Fprint(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>Feed</title><link>https://example.com</link><description>Test</description></channel></rss>`)
		}
	}))
	defer server.Close()
	client := fixtureFeedClient(t, server.Listener.Addr().String())
	parser := gofeed.NewParser()
	if _, err := fetchSourceFeed(context.Background(), client, parser, "http://feed.example/feed"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/large", "/compressed"} {
		if _, err := fetchSourceFeed(context.Background(), client, parser, "http://feed.example"+path); !errors.Is(err, gofeed.ErrResponseTooLarge) {
			t.Fatalf("oversized response path=%s err=%v", path, err)
		}
	}
	if _, err := fetchSourceFeed(context.Background(), client, parser, "http://feed.example/headers"); err == nil {
		t.Fatal("oversized headers accepted")
	}
	if _, err := fetchSourceFeed(context.Background(), client, parser, "http://feed.example/missing"); err == nil {
		t.Fatal("non-success status accepted")
	}
	client.Timeout = 30 * time.Millisecond
	if _, err := fetchSourceFeed(context.Background(), client, parser, "http://feed.example/slow"); err == nil {
		t.Fatal("slow response accepted")
	} else if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
		t.Fatalf("expected timeout, got %v", err)
	}
}

func TestFeedClientRechecksDNSOnNewConnections(t *testing.T) {
	var hits, lookups, dials atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Connection", "close")
		fmt.Fprint(w, "public response")
	}))
	defer server.Close()
	client := newFeedHTTPClient(publicFeedDialer{
		lookup: func(context.Context, string, string) ([]netip.Addr, error) {
			if lookups.Add(1) == 1 {
				return []netip.Addr{netip.MustParseAddr("1.1.1.1")}, nil
			}
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		},
		dial: func(ctx context.Context, network, target string) (net.Conn, error) {
			dials.Add(1)
			if target != "1.1.1.1:80" {
				return nil, fmt.Errorf("unexpected target %s", target)
			}
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
	})
	defer client.CloseIdleConnections()
	response, err := client.Get("http://feed.example/feed")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response, err := client.Get("http://feed.example/feed"); !errors.Is(err, ErrBlockedFeedAddress) {
		if response != nil {
			response.Body.Close()
		}
		t.Fatalf("changed DNS was not rejected: %v", err)
	}
	if hits.Load() != 1 || lookups.Load() != 2 || dials.Load() != 1 {
		t.Fatalf("hits=%d lookups=%d dials=%d", hits.Load(), lookups.Load(), dials.Load())
	}
}

func TestFeedClientHTTPSPreservesHostnameAndCertificateChecks(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Host != "example.com" || r.TLS.ServerName != "example.com" {
			t.Errorf("hostname lost: Host=%s SNI=%s", r.Host, r.TLS.ServerName)
		}
		if r.URL.Path == "/downgrade" {
			http.Redirect(w, r, "http://example.com/feed", http.StatusFound)
			return
		}
		fmt.Fprint(w, "HTTPS response")
	}))
	defer server.Close()
	client := fixtureFeedClient(t, server.Listener.Addr().String())
	transport := client.Transport.(*validatedFeedTransport).transport
	transport.TLSClientConfig = server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	if transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("fixture disabled TLS verification")
	}
	response, err := client.Get("https://example.com/feed")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response, err := client.Get("https://example.com/downgrade"); err == nil {
		response.Body.Close()
		t.Fatal("HTTPS downgrade accepted")
	}
	before := hits.Load()
	if response, err := client.Get("https://wrong.invalid/feed"); err == nil {
		response.Body.Close()
		t.Fatal("wrong-host certificate accepted")
	}
	if hits.Load() != before {
		t.Fatal("invalid TLS hostname reached handler")
	}
}

func TestCreateFeedRejectsSavedPrivateSources(t *testing.T) {
	store, db := feedTestStore(t)
	if _, err := db.Exec(`UPDATE user_sources SET is_active=0`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.InsertSources(ctx, 1, []database.SourcesUpdate{{Name: "Unsafe saved source", URL: "http://169.254.169.254/feed", IsActive: true}}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM articles`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("unsafe saved source inserted articles: count=%d err=%v", count, err)
	}
}
