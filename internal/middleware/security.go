package middleware

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"golang.org/x/time/rate"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/pkg/message"
)

// CORS middleware with billing service specific settings
func CORS(allowedOrigins []string) gin.HandlerFunc {
	corsConfig := cors.DefaultConfig()

	// If no origins specified, allow all (development mode)
	if len(allowedOrigins) == 0 {
		corsConfig.AllowAllOrigins = true
	} else {
		corsConfig.AllowOrigins = allowedOrigins
	}

	// Billing service specific headers
	corsConfig.AllowHeaders = append(corsConfig.AllowHeaders,
		"Authorization",
		"X-Request-ID",
		"X-Forwarded-For",
		"X-Real-IP",
		"X-Idempotency-Key",
	)

	corsConfig.AllowMethods = []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodOptions,
	}

	corsConfig.ExposeHeaders = []string{
		"X-Request-ID",
		"X-RateLimit-Remaining",
		"X-RateLimit-Reset",
	}

	corsConfig.AllowCredentials = true
	corsConfig.MaxAge = 12 * time.Hour

	return cors.New(corsConfig)
}

// RateLimitStore holds rate limiters for different IPs
type RateLimitStore struct {
	limiters map[string]*rate.Limiter
	config   *config.RateLimitConfig
}

// NewRateLimitStore creates a new rate limit store
func NewRateLimitStore(config *config.RateLimitConfig) *RateLimitStore {
	return &RateLimitStore{
		limiters: make(map[string]*rate.Limiter),
		config:   config,
	}
}

// RateLimit middleware implements rate limiting per IP address
func RateLimit(rateLimiterConfig *config.RateLimitConfig) gin.HandlerFunc {
	store := NewRateLimitStore(rateLimiterConfig)

	return func(c *gin.Context) {
		// Get client IP
		clientIP := getClientIP(c)

		// Get appropriate rate limit based on endpoint
		var limit *config.RateLimit

		path := c.Request.URL.Path
		switch {
		case strings.Contains(path, "/subscriptions/") && c.Request.Method == http.MethodPost:
			// Very strict for subscription creation
			limit = &config.RateLimit{
				RequestsPerMinute: 10,
				BurstSize:         3,
			}
		case strings.Contains(path, "/webhook/"):
			// Higher limit for webhooks
			limit = &config.RateLimit{
				RequestsPerMinute: 100,
				BurstSize:         20,
			}
		case strings.Contains(path, "/payment-methods/"):
			// Moderate limit for payment methods
			limit = &config.RateLimit{
				RequestsPerMinute: 20,
				BurstSize:         5,
			}
		default:
			// Use default limit
			if rateLimiterConfig != nil && rateLimiterConfig.DefaultLimit != nil {
				limit = rateLimiterConfig.DefaultLimit
			} else {
				limit = &config.RateLimit{
					RequestsPerMinute: 60,
					BurstSize:         10,
				}
			}
		}

		// Get or create rate limiter for this IP
		limiter := store.getLimiterForRate(clientIP, limit)

		// Check if request is allowed
		if !limiter.Allow() {
			log.WithFields(log.Fields{
				"client_ip": clientIP,
				"path":      path,
				"method":    c.Request.Method,
			}).Warn("Rate limit exceeded")

			c.Header("X-RateLimit-Remaining", "0")
			c.Header("Retry-After", "60")
			c.JSON(http.StatusTooManyRequests, message.Message("Rate limit exceeded"))
			c.Abort()
			return
		}

		// Add rate limit headers
		c.Header("X-RateLimit-Remaining", "1") // Simplified

		c.Next()
	}
}

// getLimiter gets or creates a rate limiter for an IP
func (s *RateLimitStore) getLimiter(ip string, config *config.RateLimitConfig) *rate.Limiter {
	limiter, exists := s.limiters[ip]
	if !exists {
		// Create new rate limiter with default values
		// This is a simplified version - in production you'd use config values
		limiter = rate.NewLimiter(rate.Limit(1), 60) // 1 req/sec, burst of 60
		s.limiters[ip] = limiter
	}
	return limiter
}

func (s *RateLimitStore) getLimiterForRate(ip string, rateCfg *config.RateLimit) *rate.Limiter {
	limiter, exists := s.limiters[ip]
	if !exists {
		// Convert requests per minute to requests per second
		rps := float64(rateCfg.RequestsPerMinute) / 60.0
		limiter = rate.NewLimiter(rate.Limit(rps), rateCfg.BurstSize)
		s.limiters[ip] = limiter
	}
	return limiter
}

// InternalIPWhitelist restricts access to internal networks only (not used by default)
func InternalIPWhitelist() gin.HandlerFunc {
	// Define internal network ranges
	internalNetworks := []*net.IPNet{
		{IP: net.ParseIP("10.0.0.0"), Mask: net.CIDRMask(8, 32)},     // 10.0.0.0/8
		{IP: net.ParseIP("172.16.0.0"), Mask: net.CIDRMask(12, 32)},  // 172.16.0.0/12
		{IP: net.ParseIP("192.168.0.0"), Mask: net.CIDRMask(16, 32)}, // 192.168.0.0/16
		{IP: net.ParseIP("127.0.0.0"), Mask: net.CIDRMask(8, 32)},    // 127.0.0.0/8 (loopback)
	}

	return func(c *gin.Context) {
		clientIP := getClientIP(c)

		// Parse client IP
		ip := net.ParseIP(clientIP)
		if ip == nil {
			log.WithField("client_ip", clientIP).Error("Failed to parse client IP")
			c.JSON(http.StatusForbidden, message.Message("Access denied"))
			c.Abort()
			return
		}

		// Check if IP is in internal networks
		isInternal := false
		for _, network := range internalNetworks {
			if network.Contains(ip) {
				isInternal = true
				break
			}
		}

		if !isInternal {
			log.WithField("client_ip", clientIP).Warn("External IP attempted to access internal endpoint")
			c.JSON(http.StatusForbidden, message.Message("Access denied"))
			c.Abort()
			return
		}

		c.Next()
	}
}

// WebhookIPWhitelist middleware restricts webhook endpoints to allowed IPs
func WebhookIPWhitelist(allowedIPs []string) gin.HandlerFunc {
	// Parse allowed IPs and networks
	var allowedNetworks []*net.IPNet
	var allowedAddresses []net.IP

	for _, ipStr := range allowedIPs {
		if strings.Contains(ipStr, "/") {
			// CIDR notation
			_, network, err := net.ParseCIDR(ipStr)
			if err != nil {
				log.WithError(err).WithField("ip", ipStr).Error("Failed to parse CIDR")
				continue
			}
			allowedNetworks = append(allowedNetworks, network)
		} else {
			// Single IP address
			ip := net.ParseIP(ipStr)
			if ip == nil {
				log.WithField("ip", ipStr).Error("Failed to parse IP address")
				continue
			}
			allowedAddresses = append(allowedAddresses, ip)
		}
	}

	return func(c *gin.Context) {
		clientIP := getClientIP(c)

		// Parse client IP
		ip := net.ParseIP(clientIP)
		if ip == nil {
			log.WithField("client_ip", clientIP).Error("Failed to parse client IP")
			c.JSON(http.StatusForbidden, message.Message("Access denied"))
			c.Abort()
			return
		}

		// Check against allowed addresses
		for _, allowedIP := range allowedAddresses {
			if ip.Equal(allowedIP) {
				c.Next()
				return
			}
		}

		// Check against allowed networks
		for _, network := range allowedNetworks {
			if network.Contains(ip) {
				c.Next()
				return
			}
		}

		log.WithField("client_ip", clientIP).Warn("Webhook request from non-whitelisted IP")
		c.JSON(http.StatusForbidden, message.Message("Access denied"))
		c.Abort()
	}
}

// SecurityHeaders adds security headers to responses
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Security headers for billing service
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; font-src 'self'")

		// Remove server information
		c.Header("Server", "")

		c.Next()
	}
}

// getClientIP extracts the real client IP from the request
func getClientIP(c *gin.Context) string {
	// Check X-Forwarded-For header (from load balancers)
	if xForwardedFor := c.GetHeader("X-Forwarded-For"); xForwardedFor != "" {
		ips := strings.Split(xForwardedFor, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header (from reverse proxies)
	if xRealIP := c.GetHeader("X-Real-IP"); xRealIP != "" {
		return xRealIP
	}

	// Fallback to connection remote address
	return c.ClientIP()
}
