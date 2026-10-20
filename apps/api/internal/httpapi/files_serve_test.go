package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestServeUploadSniffsAndSandboxes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	cases := []struct {
		name, body, wantType, wantDisp string
		wantCSP                        bool
	}{
		{"x.html", "<!doctype html><script>alert(1)</script>", "application/octet-stream", "attachment", true},
		{"x.svg", `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`, "application/octet-stream", "attachment", true},
		// Extension lies: HTML bytes in a .png still download.
		{"fake.png", "<html><body onload=alert(1)>", "application/octet-stream", "attachment", true},
		{"a.png", string(png), "image/png", "inline", true},
		{"notes.txt", "just text", "text/plain; charset=utf-8", "inline", true},
		{"doc.pdf", "%PDF-1.7\n", "application/pdf", "inline", false},
	}
	dir := t.TempDir()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name)
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/files/"+tc.name, nil)
			serveUpload(c, path, `evil"\`+"\r\n"+tc.name)

			if w.Code != http.StatusOK {
				t.Fatalf("status %d", w.Code)
			}
			if got := w.Header().Get("Content-Type"); got != tc.wantType {
				t.Errorf("Content-Type = %q, want %q", got, tc.wantType)
			}
			disp := w.Header().Get("Content-Disposition")
			if !strings.HasPrefix(disp, tc.wantDisp+";") {
				t.Errorf("Content-Disposition = %q, want %s", disp, tc.wantDisp)
			}
			if strings.ContainsAny(strings.TrimSuffix(strings.SplitN(disp, `filename="`, 2)[1], `"`), "\"\\\r\n") {
				t.Errorf("filename not sanitized: %q", disp)
			}
			if csp := w.Header().Get("Content-Security-Policy"); (csp == uploadCSP) != tc.wantCSP {
				t.Errorf("CSP = %q, want sandbox=%v", csp, tc.wantCSP)
			}
			if w.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Error("missing nosniff")
			}
		})
	}
}
