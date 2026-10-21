package httpapi

// Link unfurling: server-side fetch of <title>, favicon and og:image for a
// URL. SSRF-guarded: http(s) only, private/loopback addresses refused on
// every connection, including each redirect hop (outbound.go).

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// unfurlClient also fetches feeds; see outbound.go for the address guard.
var unfurlClient = newOutboundClient(8*time.Second, isBlockedIP)

// checkURL refuses non-http schemes and hosts that resolve to blocked
// addresses. It gives an early, readable error; the dialer guard in
// newOutboundClient is what enforces it on the connection actually made.
func checkURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("scheme not allowed")
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("missing host")
	}
	ips, err := net.DefaultResolver.LookupIPAddr(context.Background(), host)
	if err != nil || len(ips) == 0 {
		return errors.New("host unresolvable")
	}
	for _, ipa := range ips {
		if isBlockedIP(ipa.IP) {
			return errPrivateAddress
		}
	}
	return nil
}

var (
	reTitle   = regexp.MustCompile(`(?is)<title[^>]*>\s*(.*?)\s*</title>`)
	reOG      = regexp.MustCompile(`(?is)<meta[^>]+(?:property|name)=["'](?:og:title|twitter:title)["'][^>]+content=["']([^"']+)["']`)
	reOGRev   = regexp.MustCompile(`(?is)<meta[^>]+content=["']([^"']+)["'][^>]+(?:property|name)=["'](?:og:title|twitter:title)["']`)
	reIcon    = regexp.MustCompile(`(?is)<link[^>]+rel=["'](?:icon|shortcut icon|apple-touch-icon)["'][^>]+href=["']([^"']+)["']`)
	reDesc    = regexp.MustCompile(`(?is)<meta[^>]+name=["']description["'][^>]+content=["']([^"']+)["']`)
	reOGImage = regexp.MustCompile(`(?is)<meta[^>]+(?:property|name)=["'](?:og:image|twitter:image)["'][^>]+content=["']([^"']+)["']`)
)

type linkPreview struct {
	Title       string
	Favicon     string
	Description string
	Image       string
}

// unfurlURL fetches a page and extracts title/favicon/description/og:image.
func unfurlURL(ctx context.Context, raw string) (linkPreview, error) {
	var out linkPreview
	u, err := url.Parse(raw)
	if err != nil || checkURL(u) != nil {
		return out, errors.New("invalid url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("User-Agent", "Cal-LinkPreview/1.0")
	req.Header.Set("Accept", "text/html")
	resp, err := unfurlClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return out, errors.New("fetch failed")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return out, err
	}
	html := string(body)
	out.Title = decodeEntities(strings.TrimSpace(first(reOG.FindStringSubmatch(html), reOGRev.FindStringSubmatch(html), reTitle.FindStringSubmatch(html))))
	if m := reIcon.FindStringSubmatch(html); m != nil {
		out.Favicon = resolveURL(u, m[1])
	}
	if out.Favicon == "" {
		out.Favicon = u.Scheme + "://" + u.Host + "/favicon.ico"
	}
	if m := reOGImage.FindStringSubmatch(html); m != nil {
		out.Image = resolveURL(u, m[1])
	}
	if m := reDesc.FindStringSubmatch(html); m != nil {
		out.Description = decodeEntities(strings.TrimSpace(m[1]))
	}
	return out, nil
}

func (s *Server) unfurl(c *gin.Context) {
	p, err := unfurlURL(c.Request.Context(), strings.TrimSpace(c.Query("url")))
	if err != nil {
		c.String(http.StatusBadGateway, "fetch failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"title":       p.Title,
		"favicon":     p.Favicon,
		"description": p.Description,
		"image":       p.Image,
	})
}

func first(ms ...[]string) string {
	for _, m := range ms {
		if len(m) > 1 && strings.TrimSpace(m[1]) != "" {
			return m[1]
		}
	}
	return ""
}

func resolveURL(base *url.URL, href string) string {
	r, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return base.ResolveReference(r).String()
}

var reEntity = regexp.MustCompile(`&(?:#(\d+)|amp|quot|lt|gt|apos|nbsp);`)

func decodeEntities(s string) string {
	return reEntity.ReplaceAllStringFunc(s, func(ent string) string {
		switch ent {
		case "&amp;":
			return "&"
		case "&quot;":
			return `"`
		case "&lt;":
			return "<"
		case "&gt;":
			return ">"
		case "&apos;":
			return "'"
		case "&nbsp;":
			return " "
		}
		if m := reEntity.FindStringSubmatch(ent); m != nil && m[1] != "" {
			if code, err := strconv.Atoi(m[1]); err == nil && code > 0 && code < 0x10FFFF {
				return string(rune(code))
			}
		}
		return ent
	})
}
