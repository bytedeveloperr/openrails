package authprovider

import (
	"github.com/gin-gonic/gin"
)

// Provider is the app-facing auth boundary for verification middleware.
// Billing is a verifier-only service; it does not mount AuthKit routes or mint tokens.
//
// The middleware must set the user context in the Gin context using:
//
//	c.Set("billing.user_context", authprovider.UserContext{...})
//	c.Request = c.Request.WithContext(authprovider.SetUserContext(ctx, uc))
//
// Handlers then retrieve user context via authprovider.UserContextFromGin(c).
//
// Implementations:
//   - AuthKitProvider (internal/auth/provider.go) - wraps JWT verifier for standalone mode
//   - Custom providers - implement this interface to use your own auth system in embedded mode
//
// Example custom implementation:
//
//	type MyAuthProvider struct { ... }
//	func (p *MyAuthProvider) Required() gin.HandlerFunc {
//	    return func(c *gin.Context) {
//	        // Validate auth, extract user info
//	        uc := authprovider.UserContext{UserID: "...", Email: "..."}
//	        c.Set("billing.user_context", uc)
//	        c.Request = c.Request.WithContext(authprovider.SetUserContext(c.Request.Context(), uc))
//	        c.Next()
//	    }
//	}
//	func (p *MyAuthProvider) Optional() gin.HandlerFunc { ... }
type Provider interface {
	// Required returns middleware that requires authentication.
	// Requests without valid auth will be rejected with 401.
	// Must set UserContext in Gin context on success.
	Required() gin.HandlerFunc

	// Optional returns middleware that attempts authentication but allows unauthenticated requests.
	// UserContext will be empty for unauthenticated requests.
	Optional() gin.HandlerFunc
}
