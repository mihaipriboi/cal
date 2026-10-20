package httpapi

// File attachments: POST /api/files stores under DATA_DIR/uploads/<user>/,
// GET /api/files/<name> serves only the caller's own files. Notes reference
// them as ![name](/api/files/<name>) in markdown.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"cal/apps/api/internal/store"

	"github.com/gin-gonic/gin"
)

const maxUploadBytes = 20 << 20 // 20 MB

var reSafeName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,80}$`)

func (s *Server) uploadFile(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)
	header, err := c.FormFile("file")
	if err != nil {
		c.String(http.StatusBadRequest, "file required (max 20 MB)")
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if len(ext) > 10 {
		ext = ext[:10]
	}
	// Generated name only — never trust the client-supplied filename for a path.
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	name := hex.EncodeToString(buf) + ext

	dir := filepath.Join(s.dataDir, "uploads", currentUser(c).ID)
	dst := filepath.Join(dir, name)
	// Quota: settings.quota_mb caps the user's upload dir.
	settings, err := s.store.Settings(c.Request.Context(), currentUser(c).ID)
	if err == nil && settings.QuotaMB > 0 {
		if dirSize(dir)+header.Size > int64(settings.QuotaMB)<<20 {
			c.String(http.StatusInsufficientStorage, "storage quota exceeded")
			return
		}
	}
	// Optional person attachment — validate ownership before writing anything.
	personID := nilIfEmptyStr(c.PostForm("personId"))
	if personID != nil {
		if _, err := s.store.GetPerson(c.Request.Context(), currentUser(c).ID, *personID); err != nil {
			c.String(http.StatusBadRequest, "person not found")
			return
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		c.String(http.StatusInternalServerError, "storage unavailable")
		return
	}
	if err := c.SaveUploadedFile(header, dst); err != nil {
		c.String(http.StatusInternalServerError, "save failed")
		return
	}
	mime := header.Header.Get("Content-Type")
	if mime == "" {
		mime = "application/octet-stream"
	}
	sha := sha256File(dst)
	// Dedup: an identical binary already stored for this user wins — drop the
	// fresh copy, keep the original row (fill person_id if the caller asked).
	if sha != "" {
		if dup, err := s.store.FileByHash(c.Request.Context(), currentUser(c).ID, sha); err == nil {
			_ = os.Remove(dst)
			if personID != nil {
				_ = s.store.AttachFileToPerson(c.Request.Context(), currentUser(c).ID, dup.ID, *personID)
			}
			c.JSON(http.StatusOK, gin.H{
				"id":       dup.ID,
				"url":      "/api/files/" + dup.Name,
				"name":     header.Filename,
				"markdown": "![" + header.Filename + "](/api/files/" + dup.Name + ")",
				"deduped":  true,
			})
			return
		}
	}
	rec, err := s.store.CreateFile(c.Request.Context(), currentUser(c).ID, name, header.Filename, mime, header.Size, sha, splitTags(c.PostForm("tags")), nilIfEmptyStr(c.PostForm("workspaceId")), personID)
	if err != nil {
		c.String(http.StatusInternalServerError, "record failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":       rec.ID,
		"url":      "/api/files/" + name,
		"name":     header.Filename,
		"markdown": "![" + header.Filename + "](/api/files/" + name + ")",
	})
}

// sha256File hashes a stored binary; "" on error (hash is best-effort).
func sha256File(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// listFiles returns the caller's uploads, newest first.
func (s *Server) listFiles(c *gin.Context) {
	files, err := s.store.ListFiles(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, files)
}

// deleteFile removes the row then the disk file.
func (s *Server) deleteFile(c *gin.Context) {
	userID := currentUser(c).ID
	name, err := s.store.DeleteFile(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	_ = os.Remove(filepath.Join(s.dataDir, "uploads", userID, name))
	c.Status(http.StatusNoContent)
}

// shareFile toggles the public share token: on → generates one, off → clears.
func (s *Server) shareFile(c *gin.Context) {
	var body struct {
		On bool `json:"on"`
	}
	_ = c.ShouldBindJSON(&body)
	var token *string
	if body.On {
		buf := make([]byte, 20)
		_, _ = rand.Read(buf)
		t := hex.EncodeToString(buf)
		token = &t
	}
	if err := s.store.SetFileShare(c.Request.Context(), currentUser(c).ID, c.Param("id"), token); errors.Is(err, store.ErrNotFound) {
		c.String(http.StatusNotFound, "not found")
		return
	} else if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	if token == nil {
		c.JSON(http.StatusOK, gin.H{"shareToken": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"shareToken": *token})
}

// serveSharedFile resolves /api/shared/files/<token> without a session.
func (s *Server) serveSharedFile(c *gin.Context) {
	userID, f, err := s.store.FileByShareToken(c.Request.Context(), c.Param("token"))
	if err != nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	path := filepath.Join(s.dataDir, "uploads", userID, f.Name)
	if _, err := os.Stat(path); err != nil {
		c.String(http.StatusNotFound, "file gone")
		return
	}
	serveUpload(c, path, f.OrigName)
}

// dirSize totals a directory (one level — uploads are flat per user).
func dirSize(dir string) int64 {
	var total int64
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil {
			total += info.Size()
		}
	}
	return total
}

// storageUsage reports used/limit for the settings UI.
func (s *Server) storageUsage(c *gin.Context) {
	userID := currentUser(c).ID
	settings, err := s.store.Settings(c.Request.Context(), userID)
	limit := int64(500) << 20
	if err == nil && settings.QuotaMB > 0 {
		limit = int64(settings.QuotaMB) << 20
	}
	c.JSON(http.StatusOK, gin.H{
		"usedBytes":  dirSize(filepath.Join(s.dataDir, "uploads", userID)),
		"quotaBytes": limit,
	})
}

func (s *Server) serveFile(c *gin.Context) {
	name := c.Param("name")
	if !reSafeName.MatchString(name) {
		c.String(http.StatusNotFound, "not found")
		return
	}
	path := filepath.Join(s.dataDir, "uploads", currentUser(c).ID, name)
	if _, err := os.Stat(path); err != nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	serveUpload(c, path, name)
}

// inlineTypes are sniffed types safe to render on the app origin: images,
// media, PDF and plain text. Anything else (HTML, SVG, XML, scripts) is
// served as an opaque download so an uploaded file can never run as the app.
var inlineTypes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true,
	"image/bmp": true, "image/x-icon": true,
	"video/mp4": true, "video/webm": true, "audio/mpeg": true, "audio/wave": true,
	"audio/ogg": true, "application/ogg": true, "audio/aiff": true,
	"application/pdf": true, "text/plain": true,
}

// uploadCSP sandboxes inline uploads: no scripts, no same-origin access.
// PDFs skip it because browsers refuse to run their PDF viewer in a sandbox.
const uploadCSP = "sandbox; default-src 'none'; img-src 'self' data:; media-src 'self'; style-src 'unsafe-inline'"

// serveUpload picks the Content-Type from the file's bytes, not its name or
// stored MIME, and forces a download for anything outside inlineTypes.
func serveUpload(c *gin.Context, path, filename string) {
	f, err := os.Open(path)
	if err != nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	f.Close()
	mime, _, _ := strings.Cut(http.DetectContentType(head[:n]), ";")
	disposition := "inline"
	switch {
	case !inlineTypes[mime]:
		mime, disposition = "application/octet-stream", "attachment"
	case mime == "text/plain":
		mime = "text/plain; charset=utf-8"
	}
	if mime != "application/pdf" {
		c.Header("Content-Security-Policy", uploadCSP)
	}
	safeName := strings.NewReplacer(`"`, "", `\`, "", "\r", "", "\n", "").Replace(filename)
	c.Header("Content-Type", mime)
	c.Header("Content-Disposition", disposition+`; filename="`+safeName+`"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.File(path)
}
