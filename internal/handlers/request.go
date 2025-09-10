package handlers

import (
	"mime/multipart"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/internal/state"
	"github.com/doujins-org/doujins-billing/pkg/message"
)

type Request struct {
	State   *state.State
	GinCtx  *gin.Context
	Request *http.Request
}

func NewRequest(ctx *gin.Context, state *state.State) *Request {
	return &Request{
		GinCtx:  ctx,
		State:   state,
		Request: ctx.Request,
	}
}

func (r *Request) AbortJSON(code int, message string) {
	r.GinCtx.AbortWithStatusJSON(code, message)
}

func (r *Request) ErrorJSON(code int, msg string) {
	r.GinCtx.JSON(code, message.Message(msg))
}

func (r *Request) SuccessJSON(data any) {
	r.GinCtx.JSON(http.StatusOK, data)
}

func (r *Request) SuccessJSONMessage(msg string) {
	r.GinCtx.JSON(http.StatusOK, message.Json{
		"message": msg,
	})
}

func (r *Request) Bind(data any) error {
	return r.GinCtx.Bind(data)
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

func (r *Request) GetState() *state.State {
	return r.State
}
