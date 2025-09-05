package middleware

import (
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/doujins-org/doujins-billing/config"
)

// InternalOnly enforces a shared secret for admin/private endpoints.
// Looks for the secret in header 'X-API-KEY'. If no token is configured,
// admin is considered disabled and requests are rejected.
func InternalOnly(adminCfg *config.AdminConfig) gin.HandlerFunc {
    return func(c *gin.Context) {
        if adminCfg == nil || adminCfg.APIKey == "" {
            c.JSON(http.StatusServiceUnavailable, gin.H{"error": "admin access disabled"})
            c.Abort()
            return
        }
        token := c.GetHeader("X-API-KEY")
        if token == "" || token != adminCfg.APIKey {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
            c.Abort()
            return
        }
        c.Next()
    }
}
