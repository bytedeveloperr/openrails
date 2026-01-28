package policy

import (
	"context"

	"github.com/doujins-org/doujins-billing/pkg/authprovider"
	"github.com/doujins-org/ginapi/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"
)

func IsAdmin(ctx context.Context, db bun.IDB, userID string) (bool, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return false, nil
	}

	// Always check admin role live in Postgres (immediate revocation).
	return db.NewSelect().
		TableExpr("profiles.user_roles ur").
		Join("JOIN profiles.roles r ON ur.role_id = r.id").
		Where("ur.user_id = ? AND r.slug = 'admin' AND r.deleted_at IS NULL AND EXISTS (SELECT 1 FROM profiles.users u WHERE u.id = ? AND u.deleted_at IS NULL AND u.banned_at IS NULL)", uid, uid).
		Exists(ctx)
}

// AdminRequired ensures the current authenticated user has the "admin" role.
// This policy is app-specific; it should not live in authkit.
func AdminRequired(db bun.IDB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uc, ok := authprovider.UserContextFromGin(c)
		if !ok || uc.UserID == "" {
			response.UnauthorizedWithMessage(c, "authentication required")
			c.Abort()
			return
		}
		isAdmin, err := IsAdmin(c.Request.Context(), db, uc.UserID)
		if err != nil {
			log.WithError(err).Error("failed to check admin role")
			response.InternalError(c, "failed to check admin role")
			c.Abort()
			return
		}
		if !isAdmin {
			log.WithField("user_id", uc.UserID).Warn("admin access denied")
			response.ForbiddenWithMessage(c, "admin privileges required")
			c.Abort()
			return
		}
		c.Next()
	}
}
