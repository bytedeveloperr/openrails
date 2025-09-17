package middleware

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/oidc"
	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/pkg/message"
)

// UserContextKey is the key for user context in gin.Context
const UserContextKey = "user"
const UserIDContextKey = "user_id"

// UserContext represents the authenticated user context
type UserContext struct {
	User      *services.UserIdentity `json:"user"`
	SessionID string                 `json:"session_id"`
	ExpiresAt int64                  `json:"exp"`
}

// HasRole checks if the user has a specific role
func (u *UserContext) HasRole(role string) bool {
	for _, r := range u.User.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// ExtractUserContextFromClaims extracts user context from JWT claims
func ExtractUserContextFromClaims(claims jwt.MapClaims) (*UserContext, error) {
	userCtx := &UserContext{}

	userCtx.User = &services.UserIdentity{}

	// Extract user ID
	if userID, ok := claims["sub"].(string); ok {
		userCtx.User.ID = userID
	}

	// Extract email
	if email, ok := claims["email"].(string); ok {
		emailCopy := email
		userCtx.User.Email = &emailCopy
	}

	// Extract username: prefer preferred_username, then username, then name/displayName
	if username, ok := claims["preferred_username"].(string); ok {
		userCtx.User.Username = username
	} else if username, ok := claims["username"].(string); ok {
		userCtx.User.Username = username
	} else if username, ok := claims["name"].(string); ok {
		userCtx.User.Username = username
	} else if username, ok := claims["displayName"].(string); ok {
		userCtx.User.Username = username
	}

	// Extract roles
	if rolesInterface, ok := claims["roles"].([]interface{}); ok {
		roles := make([]string, 0, len(rolesInterface))
		for _, r := range rolesInterface {
			if role, ok := r.(string); ok {
				roles = append(roles, role)
			}
		}
		userCtx.User.Roles = roles
	}

	// Extract session ID
	if sessionID, ok := claims["session_id"].(string); ok {
		userCtx.SessionID = sessionID
	}

	// Extract expiration
	if exp, ok := claims["exp"].(float64); ok {
		userCtx.ExpiresAt = int64(exp)
	}

	return userCtx, nil
}

// AuthRequired middleware verifies JWT tokens and sets user context
func AuthRequired(jwtConfig *config.JWTConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, message.Message("Authorization header required"))
			c.Abort()
			return
		}

		// Remove "Bearer " prefix
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, message.Message("Bearer token required"))
			c.Abort()
			return
		}

		// Parse and validate token
		userCtx, err := validateJWTToken(tokenString, jwtConfig)
		if err != nil {
			log.WithError(err).Warn("Invalid JWT token")
			c.JSON(http.StatusUnauthorized, message.Message("Invalid or expired token"))
			c.Abort()
			return
		}

		fmt.Println(userCtx)

		// Set user context
		c.Set(UserContextKey, userCtx)

		// Add user ID to request context for logging
		//lint:ignore SA1029 safe to use string as key
		ctx := context.WithValue(c.Request.Context(), UserIDContextKey, userCtx.User.ID)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// AdminRequired middleware ensures the user has admin privileges
func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user context (should be set by AuthRequired middleware)
		userCtxInterface, exists := c.Get(UserContextKey)
		if !exists {
			c.JSON(http.StatusUnauthorized, message.Message("Authentication required"))
			c.Abort()
			return
		}

		userCtx, ok := userCtxInterface.(*UserContext)
		if !ok {
			c.JSON(http.StatusInternalServerError, message.Message("Invalid user context"))
			c.Abort()
			return
		}

		// Check if user has admin role
		if !userCtx.HasRole("admin") {
			log.WithField(UserIDContextKey, userCtx.User.ID).Warn("Admin access denied")
			c.JSON(http.StatusForbidden, message.Message("Admin privileges required"))
			c.Abort()
			return
		}

		c.Next()
	}
}

// OptionalAuth middleware extracts user context if token is provided, but doesn't require it
func OptionalAuth(jwtConfig *config.JWTConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			// No token provided, continue without authentication
			c.Next()
			return
		}

		// Remove "Bearer " prefix
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			// Invalid format, continue without authentication
			c.Next()
			return
		}

		// Try to validate token
		userCtx, err := validateJWTToken(tokenString, jwtConfig)
		if err != nil {
			log.WithError(err).Debug("Optional auth token validation failed")
			// Continue without authentication
			c.Next()
			return
		}

		// Set user context if token is valid
		c.Set(UserContextKey, userCtx)

		// Add user ID to request context for logging
		//lint:ignore SA1029 safe to use string as key
		ctx := context.WithValue(c.Request.Context(), UserIDContextKey, userCtx.User.ID)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// validateJWTToken parses and validates a JWT token
var (
	jwksCachesMu  = struct{}{}
	jwksCachesMap = map[string]*oidc.JWKSCache{}
)

func validateJWTToken(tokenString string, jwtConfig *config.JWTConfig) (*UserContext, error) {
	// Parse token with dynamic keyfunc supporting:
	// - HMAC (HS256/384/512) using jwt.Secret (fits Casdoor defaults)
	// - RSA via static PublicKeyPEM
	// - RSA via JWKS discovered from Issuer (OIDC discovery)
	keyFunc := func(token *jwt.Token) (interface{}, error) {
		alg := token.Method.Alg()
		switch alg {
		case jwt.SigningMethodHS256.Alg(), jwt.SigningMethodHS384.Alg(), jwt.SigningMethodHS512.Alg():
			if jwtConfig.Secret == "" {
				return nil, fmt.Errorf("HMAC secret not configured")
			}
			return []byte(jwtConfig.Secret), nil
		case jwt.SigningMethodRS256.Alg(), jwt.SigningMethodRS384.Alg(), jwt.SigningMethodRS512.Alg():
			// Prefer static PEM if provided
			if jwtConfig.PublicKeyPEM != "" {
				block, _ := pem.Decode([]byte(jwtConfig.PublicKeyPEM))
				if block == nil {
					return nil, fmt.Errorf("invalid PEM for JWT public key")
				}
				pubAny, err := x509.ParsePKIXPublicKey(block.Bytes)
				if err != nil {
					return nil, fmt.Errorf("parse RSA public key: %w", err)
				}
				pub, ok := pubAny.(*rsa.PublicKey)
				if !ok {
					return nil, fmt.Errorf("not an RSA public key")
				}
				return pub, nil
			}
			// Else use issuer JWKS
			if jwtConfig.Issuer == "" {
				return nil, fmt.Errorf("issuer required for JWKS validation")
			}
			// Extract kid
			kid, _ := token.Header["kid"].(string)
			// Get/create cache per issuer
			// Note: simple global map without heavy locking since lookup + occasional replace is fine.
			cache := jwksCachesMap[jwtConfig.Issuer]
			if cache == nil {
				cache = oidc.NewJWKSCache(jwtConfig.Issuer)
				jwksCachesMap[jwtConfig.Issuer] = cache
			}
			ctx := context.Background()
			key, err := cache.GetKey(ctx, kid)
			if err != nil {
				return nil, err
			}
			return key, nil
		default:
			return nil, fmt.Errorf("unsupported signing algorithm: %s", alg)
		}
	}

	token, err := jwt.Parse(tokenString, keyFunc)

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	// Validate token and extract claims
	if !token.Valid {
		return nil, fmt.Errorf("token is invalid")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	fmt.Println("claims", claims)

	// Validate standard claims against config (issuer, audience, expiration)
	now := time.Now()
	fmt.Println(jwtConfig)
	if iss, ok := claims["iss"].(string); jwtConfig.Issuer != "" && (!ok || iss != jwtConfig.Issuer) {
		return nil, fmt.Errorf("invalid issuer")
	}
	if jwtConfig.Audience != "" {
		audOK := false
		switch aud := claims["aud"].(type) {
		case string:
			audOK = aud == jwtConfig.Audience
		case []interface{}:
			for _, a := range aud {
				if s, ok := a.(string); ok && s == jwtConfig.Audience {
					audOK = true
					break
				}
			}
		}
		if !audOK {
			return nil, fmt.Errorf("invalid audience")
		}
	}
	if exp, ok := claims["exp"].(float64); ok {
		if int64(exp) <= now.Unix() {
			return nil, fmt.Errorf("token expired")
		}
	}

	// Extract user information from claims
	userCtx, err := ExtractUserContextFromClaims(claims)
	if err != nil {
		return nil, fmt.Errorf("failed to extract user context: %w", err)
	}

	return userCtx, nil
}

// GetUserContext retrieves user context from gin.Context
func GetUserContext(c *gin.Context) *UserContext {
	userCtxInterface, exists := c.Get(UserContextKey)
	if !exists {
		return nil
	}

	userCtx, ok := userCtxInterface.(*UserContext)
	if !ok {
		return nil
	}

	return userCtx
}

// RequireUserContext ensures user context exists (helper for handlers)
func RequireUserContext(c *gin.Context) (*UserContext, bool) {
	userCtx := GetUserContext(c)
	if userCtx == nil {
		c.JSON(http.StatusUnauthorized, message.Message("Authentication required"))
		return nil, false
	}
	return userCtx, true
}
