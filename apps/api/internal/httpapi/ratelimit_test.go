package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// limitedRouter mirrors New(): proxies as configured, then an n/min limiter.
func limitedRouter(t *testing.T, proxies string, n int) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	list, err := trustedProxies(proxies)
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	if err := r.SetTrustedProxies(list); err != nil {
		t.Fatal(err)
	}
	r.GET("/x", newRateLimiter(n, time.Minute), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	return r
}

func hit(r http.Handler, remote, xff string) int {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = remote
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

// Without trusted proxies, rotating X-Forwarded-For must not mint new buckets.
func TestRateLimitIgnoresSpoofedForwardedFor(t *testing.T) {
	r := limitedRouter(t, "", 3)
	for i := 0; i < 3; i++ {
		if code := hit(r, "203.0.113.9:1000", "198.51.100."+strconv.Itoa(i)); code != http.StatusNoContent {
			t.Fatalf("request %d: got %d", i, code)
		}
	}
	if code := hit(r, "203.0.113.9:1000", "198.51.100.99"); code != http.StatusTooManyRequests {
		t.Fatalf("4th request with fresh XFF: got %d, want 429", code)
	}
}

// Behind a trusted proxy, each forwarded client gets its own bucket.
func TestRateLimitHonorsTrustedProxy(t *testing.T) {
	r := limitedRouter(t, "10.0.0.5", 1)
	if code := hit(r, "10.0.0.5:4000", "198.51.100.1"); code != http.StatusNoContent {
		t.Fatalf("client A: got %d", code)
	}
	if code := hit(r, "10.0.0.5:4000", "198.51.100.2"); code != http.StatusNoContent {
		t.Fatalf("client B: got %d", code)
	}
	if code := hit(r, "10.0.0.5:4000", "198.51.100.1"); code != http.StatusTooManyRequests {
		t.Fatalf("client A again: got %d, want 429", code)
	}
	// A client prepending a fake hop still lands in its real bucket.
	if code := hit(r, "10.0.0.5:4000", "1.2.3.4, 198.51.100.2"); code != http.StatusTooManyRequests {
		t.Fatalf("client B spoofing a hop: got %d, want 429", code)
	}
}

func TestTrustedProxiesParsing(t *testing.T) {
	got, err := trustedProxies(" 10.0.0.5, 192.168.1.0/24 ,,::1 ")
	if err != nil || len(got) != 3 {
		t.Fatalf("got %v, %v", got, err)
	}
	if got, err := trustedProxies(""); err != nil || got != nil {
		t.Fatalf("empty: got %v, %v", got, err)
	}
	if _, err := trustedProxies("10.0.0.300"); err == nil {
		t.Fatal("invalid IP accepted")
	}
}

func TestAllowedOrigin(t *testing.T) {
	const web = "https://cal.example.com"
	cases := map[string]bool{
		web:                                true,
		"https://cal.example.com.evil.com": false,
		"http://cal.example.com":           false,
		"capacitor://localhost":            true,
		"ionic://localhost":                true,
		"capacitor://evil.com":             false,
		"https://localhost":                true,
		"http://localhost:5173":            true,
		"http://127.0.0.1:8080":            true,
		"http://[::1]:3000":                true,
		"http://localhost.evil.com":        false,
		"https://127.0.0.1.evil.com":       false,
		"http://localhost@evil.com":        false,
		"http://user@localhost":            false,
		"null":                             false,
		"":                                 false,
		"file://localhost":                 false,
	}
	for origin, want := range cases {
		if got := allowedOrigin(origin, web); got != want {
			t.Errorf("allowedOrigin(%q) = %v, want %v", origin, got, want)
		}
	}
}
