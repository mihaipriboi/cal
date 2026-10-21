package httpapi

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "10.1.2.3", "172.16.0.1", "172.31.255.255", "192.168.0.12",
		"169.254.169.254", "100.64.0.1", "100.127.255.255", "0.0.0.0", "0.1.2.3",
		"192.0.0.8", "198.18.0.1", "224.0.0.1", "255.255.255.255",
		"::", "::1", "::127.0.0.1", "::ffff:127.0.0.1", "::ffff:192.168.0.1",
		"64:ff9b::c0a8:1", "64:ff9b:1::1", "2001::1", "2002:c0a8:1::1",
		"fc00::1", "fd12:3456::1", "fe80::1", "fec0::1", "ff02::1",
	}
	allowed := []string{
		"1.1.1.1", "8.8.8.8", "86.125.163.54", "100.63.255.255", "100.128.0.1",
		"172.32.0.1", "192.169.0.1", "::ffff:1.1.1.1", "2606:4700:4700::1111",
		"2a00:1450:4001::1",
	}
	for _, s := range blocked {
		if !isBlockedIP(net.ParseIP(s)) {
			t.Errorf("%s: want blocked", s)
		}
	}
	for _, s := range allowed {
		if isBlockedIP(net.ParseIP(s)) {
			t.Errorf("%s: want allowed", s)
		}
	}
}

// countingServer counts requests; handler may be nil for a plain 200.
func countingServer(t *testing.T, ln net.Listener, handler http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if handler != nil {
			handler(w, r)
		}
	}))
	if ln != nil {
		_ = srv.Listener.Close()
		srv.Listener = ln
	}
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, &hits
}

// The guard sits in the dialer, so a host that passed checkURL but resolves
// to a blocked address at connect time (DNS rebinding) is still refused.
func TestOutboundClientRefusesBlockedDial(t *testing.T) {
	srv, hits := countingServer(t, nil, nil)
	_, err := newOutboundClient(5*time.Second, isBlockedIP).Get(srv.URL)
	if !errors.Is(err, errPrivateAddress) {
		t.Fatalf("err = %v, want errPrivateAddress", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("blocked server was reached %d times", hits.Load())
	}
}

// A redirect from an allowed host to a blocked one is refused at the second
// dial. 127.0.0.1 stands in for a public host, 127.0.0.2 for an internal one.
func TestOutboundClientRefusesBlockedRedirect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.2:0")
	if err != nil {
		t.Skipf("no 127.0.0.2 on this host: %v", err)
	}
	internal, internalHits := countingServer(t, ln, nil)
	public, _ := countingServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL+"/hook", http.StatusTemporaryRedirect)
	})
	client := newOutboundClient(5*time.Second, func(ip net.IP) bool { return ip.Equal(net.ParseIP("127.0.0.2")) })

	resp, err := client.Post(public.URL, "application/json", strings.NewReader(`{"event":"x"}`))
	if err == nil {
		resp.Body.Close()
	}
	if !errors.Is(err, errPrivateAddress) {
		t.Fatalf("err = %v, want errPrivateAddress", err)
	}
	if internalHits.Load() != 0 {
		t.Fatalf("internal server was reached %d times", internalHits.Load())
	}
}

func TestOutboundClientRedirectLimits(t *testing.T) {
	srv, hits := countingServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ftp" {
			http.Redirect(w, r, "ftp://example.com/x", http.StatusFound)
			return
		}
		n, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/"))
		http.Redirect(w, r, "/"+strconv.Itoa(n+1), http.StatusFound)
	})
	client := newOutboundClient(5*time.Second, nil)

	_, err := client.Get(srv.URL + "/0")
	if err == nil || !strings.Contains(err.Error(), "too many redirects") {
		t.Fatalf("err = %v, want too many redirects", err)
	}
	if got := hits.Load(); got != 4 { // the request plus 3 redirects
		t.Fatalf("hits = %d, want 4", got)
	}

	_, err = client.Get(srv.URL + "/ftp")
	if err == nil || !strings.Contains(err.Error(), "scheme not allowed") {
		t.Fatalf("err = %v, want scheme not allowed", err)
	}
}

func TestDeliverWebhookPrivateOptOut(t *testing.T) {
	var gotSig string
	srv, hits := countingServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Cal-Signature")
	})
	ctx := context.Background()

	t.Setenv("CAL_ALLOW_PRIVATE_WEBHOOKS", "")
	if err := deliverWebhook(ctx, srv.URL, "secret", "entry.created", []byte(`{}`)); err == nil {
		t.Fatal("loopback webhook delivered without opt-out")
	}
	if hits.Load() != 0 {
		t.Fatalf("server reached %d times without opt-out", hits.Load())
	}

	t.Setenv("CAL_ALLOW_PRIVATE_WEBHOOKS", "1")
	if err := deliverWebhook(ctx, srv.URL, "secret", "entry.created", []byte(`{}`)); err != nil {
		t.Fatalf("with opt-out: %v", err)
	}
	if hits.Load() != 1 || gotSig == "" {
		t.Fatalf("hits = %d, signature %q", hits.Load(), gotSig)
	}
}

// With the opt-out set, a feed on the LAN may also redirect within the LAN.
func TestFetchICSPrivateOptOutFollowsRedirect(t *testing.T) {
	const ics = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nEND:VCALENDAR\r\n"
	srv, _ := countingServer(t, nil, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/cal.ics", http.StatusMovedPermanently)
			return
		}
		_, _ = io.WriteString(w, ics)
	})

	t.Setenv("CAL_ALLOW_PRIVATE_FEEDS", "")
	if _, err := fetchICS(srv.URL + "/old"); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("without opt-out: err = %v, want not allowed", err)
	}

	t.Setenv("CAL_ALLOW_PRIVATE_FEEDS", "1")
	got, err := fetchICS(srv.URL + "/old")
	if err != nil {
		t.Fatalf("with opt-out: %v", err)
	}
	if got != ics {
		t.Fatalf("body = %q", got)
	}
}
