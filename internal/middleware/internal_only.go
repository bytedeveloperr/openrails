package middleware

import (
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/doujins-org/doujins-billing/config"
)

// InternalOnly enforces a shared secret for admin/private endpoints.
// Looks for the secret in header 'X-Internal-Token'. If no token is configured,
// admin is considered disabled and requests are rejected.
func InternalOnly(adminCfg *config.AdminConfig) gin.HandlerFunc {
    return func(c *gin.Context) {
        if adminCfg == nil || adminCfg.InternalToken == "" {
            c.JSON(http.StatusServiceUnavailable, gin.H{"error": "admin access disabled"})
            c.Abort()
            return
        }
        token := c.GetHeader("X-Internal-Token")
        if token == "" || token != adminCfg.InternalToken {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid internal token"})
            c.Abort()
            return
        }
        c.Next()
    }
}

