package handlers

import (
	"errors"
	"mime/multipart"
	"net"
	"net/http"
	"reflect"
	"strings"

	authgin "github.com/PaulFidika/authkit/adapters/gin"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/app"
	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/pkg/api"
	"github.com/doujins-org/doujins-billing/pkg/message"
	"github.com/jonboulle/clockwork"
)

type Request struct {
	State   *app.Runtime
	GinCtx  *gin.Context
	Request *http.Request
	Clock   clockwork.Clock
}

func NewRequest(ctx *gin.Context, runtime *app.Runtime) *Request {
	return &Request{
		GinCtx:  ctx,
		State:   runtime,
		Request: ctx.Request,
		Clock:   runtime.Clock,
	}
}

func (r *Request) AbortJSON(code int, msg string) {
	logrus.Error(msg)
	r.GinCtx.AbortWithStatusJSON(code, api.SimpleErrorResponse(code, msg))
}

// ErrorJSON sends a structured error response in Stripe's format
// The error type is inferred from the HTTP status code
func (r *Request) ErrorJSON(code int, msg string) {
	logrus.Error(msg)
	r.GinCtx.JSON(code, api.SimpleErrorResponse(code, msg))
}

// APIError sends a structured error response with full error details
// Example: r.APIError(api.InvalidParamError("price_id", "Price ID must be a valid UUID"))
func (r *Request) APIError(err *api.APIError) {
	logrus.WithFields(logrus.Fields{
		"type":   err.Type,
		"code":   err.Code,
		"param":  err.Param,
		"status": err.HTTPStatus,
	}).Error(err.Message)
	r.GinCtx.JSON(err.HTTPStatus, err.ToResponse())
}

func (r *Request) SuccessJSON(data any) {
	r.GinCtx.JSON(http.StatusOK, data)
}

func (r *Request) SuccessJSONMessage(msg string) {
	r.GinCtx.JSON(http.StatusOK, message.Json{
		"message": msg,
	})
}

func (r *Request) SuccessJSONPaginated(data any, total int64, limit, offset int) {
	// Calculate has_more based on whether there are items beyond current page
	dataLen := 0
	if slice, ok := data.([]any); ok {
		dataLen = len(slice)
	} else {
		// Use reflection for typed slices
		v := reflect.ValueOf(data)
		if v.Kind() == reflect.Slice {
			dataLen = v.Len()
		}
	}
	hasMore := int64(offset+dataLen) < total

	r.GinCtx.JSON(http.StatusOK, message.Json{
		"object":   "list",
		"data":     data,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": hasMore,
		"url":      r.Request.URL.Path,
	})
}

func (r *Request) Bind(data any) error {
	return r.GinCtx.Bind(data)
}

func (r *Request) BindJSON(data any) bool {
	if err := r.GinCtx.ShouldBindJSON(data); err != nil {
		r.ErrorJSON(http.StatusBadRequest, normaliseBindError(err))
		return false
	}
	return true
}

func (r *Request) BindQuery(data any) bool {
	if err := r.GinCtx.ShouldBindQuery(data); err != nil {
		r.ErrorJSON(http.StatusBadRequest, normaliseBindError(err))
		return false
	}
	return true
}

func (r *Request) BindURI(data any) bool {
	if err := r.GinCtx.ShouldBindUri(data); err != nil {
		r.ErrorJSON(http.StatusBadRequest, normaliseBindError(err))
		return false
	}
	return true
}

func (r *Request) Param(key string) string {
	return r.GinCtx.Param(key)
}

func (r *Request) Query(key string) string {
	return r.GinCtx.Query(key)
}

func (r *Request) Inner() *gin.Context {
	return r.GinCtx
}

func (r *Request) Next() {
	r.GinCtx.Next()
}

func (r *Request) Get(key string) (any, bool) {
	return r.GinCtx.Get(key)
}

func (r *Request) MustGet(key string) any {
	return r.GinCtx.MustGet(key)
}

func (r *Request) Set(key string, value any) {
	r.GinCtx.Set(key, value)
}

func (r *Request) GetUser() *services.UserIdentity {
	if cl, ok := authgin.ClaimsFromGin(r.GinCtx); ok && cl.UserID != "" {
		user := &services.UserIdentity{
			ID:       cl.UserID,
			Username: cl.Username,
			Roles:    cl.Roles,
		}
		if cl.Email != "" {
			email := cl.Email
			user.Email = &email
		}
		return user
	}

	user, ok := r.Get("user")
	if !ok {
		return nil
	}

	if ui, ok := user.(*services.UserIdentity); ok {
		return ui
	}

	return nil
}

func (r *Request) GetClientIP() string {
	if forwarded := r.GinCtx.GetHeader("X-Forwarded-For"); forwarded != "" {
		ips := strings.Split(forwarded, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	if realIP := r.GinCtx.GetHeader("X-Real-IP"); realIP != "" {
		return strings.TrimSpace(realIP)
	}

	ip, _, err := net.SplitHostPort(r.GinCtx.Request.RemoteAddr)
	if err != nil {
		return r.GinCtx.Request.RemoteAddr
	}

	return ip
}

func (r *Request) Redirect(code int, location string) {
	r.GinCtx.Redirect(code, location)
}

func (r *Request) FormValue(key string) string {
	return r.GinCtx.PostForm(key)
}

func (r *Request) FormFile(key string) (multipart.File, *multipart.FileHeader, error) {
	return r.GinCtx.Request.FormFile(key)
}

func (r *Request) GetState() *app.Runtime {
	return r.State
}

func normaliseBindError(err error) string {
	var verr validator.ValidationErrors
	if errors.As(err, &verr) {
		// return first field error message for brevity
		if len(verr) > 0 {
			e := verr[0]
			return strings.ToLower(e.Field()) + " is invalid"
		}
	}
	if strings.Contains(err.Error(), "EOF") {
		return "empty_request_body"
	}
	return "invalid_request"
}
