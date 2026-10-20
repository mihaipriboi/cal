package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// requireAdmin gates /api/admin/* on the caller's is_admin flag.
func (s *Server) requireAdmin(c *gin.Context) {
	if !currentUser(c).IsAdmin {
		c.String(http.StatusForbidden, "admin required")
		c.Abort()
		return
	}
	c.Next()
}

// authConfig is public — the login screen needs it to decide between the
// register form (fresh instance) and sign-in, and to hide sign-up when the
// admin closed registration.
func (s *Server) authConfig(c *gin.Context) {
	count, err := s.store.UserCount(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"hasUsers":         count > 0,
		"registrationOpen": count == 0 || s.store.AllowRegistration(c.Request.Context()),
	})
}

func (s *Server) adminUsers(c *gin.Context) {
	users, err := s.store.ListUsers(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to list users")
		return
	}
	c.JSON(http.StatusOK, users)
}

func (s *Server) adminUpdateUser(c *gin.Context) {
	id := c.Param("id")
	var input struct {
		IsAdmin *bool `json:"isAdmin"`
	}
	if !bind(c, &input) || input.IsAdmin == nil {
		c.String(http.StatusBadRequest, "invalid request")
		return
	}
	if !s.store.UserExists(c.Request.Context(), id) {
		c.String(http.StatusNotFound, "user not found")
		return
	}
	if !*input.IsAdmin {
		if admin, _ := s.store.UserIsAdmin(c.Request.Context(), id); admin {
			if n, _ := s.store.AdminCount(c.Request.Context()); n <= 1 {
				c.String(http.StatusBadRequest, "cannot demote the last admin")
				return
			}
		}
	}
	if err := s.store.SetUserAdmin(c.Request.Context(), id, *input.IsAdmin); err != nil {
		c.String(http.StatusInternalServerError, "failed to update user")
		return
	}
	// Demoting yourself invalidates the admin view client-side; return the
	// updated user so the caller can refresh its own state if needed.
	c.JSON(http.StatusOK, gin.H{"id": id, "isAdmin": *input.IsAdmin})
}

func (s *Server) adminDeleteUser(c *gin.Context) {
	id := c.Param("id")
	if id == currentUser(c).ID {
		c.String(http.StatusBadRequest, "cannot delete your own account")
		return
	}
	if !s.store.UserExists(c.Request.Context(), id) {
		c.String(http.StatusNotFound, "user not found")
		return
	}
	if admin, _ := s.store.UserIsAdmin(c.Request.Context(), id); admin {
		if n, _ := s.store.AdminCount(c.Request.Context()); n <= 1 {
			c.String(http.StatusBadRequest, "cannot delete the last admin")
			return
		}
	}
	if err := s.store.DeleteUser(c.Request.Context(), id); err != nil {
		c.String(http.StatusInternalServerError, "failed to delete user")
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) adminConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"allowRegistration": s.store.AllowRegistration(c.Request.Context()),
	})
}

func (s *Server) adminUpdateConfig(c *gin.Context) {
	var input struct {
		AllowRegistration *bool `json:"allowRegistration"`
	}
	if !bind(c, &input) || input.AllowRegistration == nil {
		c.String(http.StatusBadRequest, "invalid request")
		return
	}
	if err := s.store.SetAllowRegistration(c.Request.Context(), *input.AllowRegistration); err != nil {
		c.String(http.StatusInternalServerError, "failed to save")
		return
	}
	c.JSON(http.StatusOK, gin.H{"allowRegistration": *input.AllowRegistration})
}

// registrationClosed is the fast-path check before hashing: the first account
// always gets in (it becomes admin); afterwards the instance policy applies.
// CreateUser re-checks under a lock, so this only saves work.
func (s *Server) registrationClosed(c *gin.Context) bool {
	count, err := s.store.UserCount(c.Request.Context())
	if err != nil {
		return true // fail closed
	}
	return count > 0 && !s.store.AllowRegistration(c.Request.Context())
}
