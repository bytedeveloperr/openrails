package policy

import (
	authgin "github.com/PaulFidika/authkit/adapters/gin"
	"github.com/doujins-org/ginapi/response"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// AdminRequired ensures the current authenticated user has the "admin" role.
// This policy is app-specific; it should not live in authkit.
func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		cl, ok := authgin.ClaimsFromGin(c)
		if !ok || cl.UserID == "" {
			response.UnauthorizedWithMessage(c, "authentication required")
			c.Abort()
			return
		}
		if !cl.HasRole("admin") {
			log.WithField("user_id", cl.UserID).Warn("admin access denied")
			response.ForbiddenWithMessage(c, "admin privileges required")
			c.Abort()
			return
		}
		c.Next()
	}
}
