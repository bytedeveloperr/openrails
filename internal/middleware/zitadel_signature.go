package middleware

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "bytes"
    "io"
    "net/http"
    "strings"

    "github.com/gin-gonic/gin"
    log "github.com/sirupsen/logrus"

    "github.com/doujins-org/doujins-billing/config"
    "github.com/doujins-org/doujins-billing/pkg/message"
)

// ZitadelSignature verifies HMAC signature for ZITADEL Actions v2 HTTP calls.
// It expects an HMAC-SHA256 signature of the raw request body provided in the header
// name specified by cfg.SignatureHeader (default: "Zitadel-Signature").
//
// Header formats supported (flexible to ease integration):
//   - Raw hex signature:    ab12cd...
//   - Key=val style:        v1=ab12cd...
// If multiple comma-separated tokens are provided (e.g., "t=...,v1=..."), the first
// token with prefix "v1=" is used as the signature.
func ZitadelSignature(cfg *config.ZitadelConfig) gin.HandlerFunc {
    headerName := "Zitadel-Signature"

    return func(c *gin.Context) {
        if cfg == nil || cfg.SigningKey == "" {
            c.JSON(http.StatusServiceUnavailable, message.Message("zitadel signing not configured"))
            c.Abort()
            return
        }

        sigHeader := c.GetHeader(headerName)
        if sigHeader == "" {
            // try common variants just in case
            if v := c.GetHeader(strings.ToUpper(headerName)); v != "" { sigHeader = v }
            if v := c.GetHeader(strings.ToLower(headerName)); v != "" { sigHeader = v }
        }
        if sigHeader == "" {
            c.JSON(http.StatusUnauthorized, message.Message("missing signature"))
            c.Abort()
            return
        }

        bodyBytes, err := io.ReadAll(c.Request.Body)
        if err != nil {
            log.WithError(err).Error("failed to read request body for signature verification")
            c.JSON(http.StatusBadRequest, message.Message("invalid request body"))
            c.Abort()
            return
        }
        // restore body for downstream handlers
        c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

        mac := hmac.New(sha256.New, []byte(cfg.SigningKey))
        mac.Write(bodyBytes)
        expected := mac.Sum(nil)

        provided := sigHeader
        // Extract after '=' if present (e.g., v1=...)
        if idx := strings.Index(provided, ","); idx >= 0 {
            // pick the v1= token if present
            parts := strings.Split(provided, ",")
            for _, p := range parts {
                p = strings.TrimSpace(p)
                if strings.HasPrefix(p, "v1=") {
                    provided = strings.TrimPrefix(p, "v1=")
                    break
                }
            }
        }
        if eq := strings.Index(provided, "="); eq >= 0 {
            provided = provided[eq+1:]
        }

        // Allow either hex or base16 without 0x
        decoded, err := hex.DecodeString(strings.TrimSpace(provided))
        if err != nil {
            // If header isn't hex, compare as-is to hex(expected)
            if subtleEqual([]byte(strings.TrimSpace(provided)), []byte(hex.EncodeToString(expected))) {
                c.Next()
                return
            }
            c.JSON(http.StatusUnauthorized, message.Message("invalid signature format"))
            c.Abort()
            return
        }

        if !subtleEqual(decoded, expected) {
            c.JSON(http.StatusUnauthorized, message.Message("invalid signature"))
            c.Abort()
            return
        }

        c.Next()
    }
}

func subtleEqual(a, b []byte) bool {
    if len(a) != len(b) { return false }
    // Use constant-time compare
    return hmac.Equal(a, b)
}
