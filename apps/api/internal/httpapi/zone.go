package httpapi

import (
	"time"

	"cal/apps/api/internal/store"

	"github.com/gin-gonic/gin"
)

// userZone is the signed-in user's Settings time zone, which their dates and
// times are read and shown in (UTC when it's unset).
func (s *Server) userZone(c *gin.Context) *time.Location {
	return s.store.UserLocation(c.Request.Context(), currentUser(c).ID)
}

// localDay is today's date in loc as a UTC-midnight time, so date
// arithmetic on it (AddDate, Sub) never meets a DST change.
func localDay(loc *time.Location) time.Time {
	y, m, d := time.Now().In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// todayIn is today's date in loc.
func todayIn(loc *time.Location) string {
	return store.Today(loc)
}
