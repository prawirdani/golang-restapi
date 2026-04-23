// Package http provides HTTP transport utilities for the application.
//
// It includes:
//   - Domain-specific HTTP handlers that delegate business logic to services (handler/*)
//   - Middleware for request processing, authentication, logging, etc. (middleware/*)
//   - Helpers for request context, JSON responses, error handling, and request body parsing
//
// Handlers should focus only on HTTP concerns and formatting responses.
// Business logic belongs in the domain/service layers. This package is internal
// and not intended for use outside the application.
package http

import (
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/prawirdani/golang-restapi/pkg/validator"
)

// Context wraps http.ResponseWriter and *http.Request with helper methods
type Context struct {
	w http.ResponseWriter
	r *http.Request
}

// Func defines the handler signature that uses the custom Context and returns an error
type Func func(c *Context) error

// JSON sends a JSON response
func (c *Context) JSON(status int, data any) error {
	// Only use ETag for successful responses (2xx)
	if status >= 200 && status < 300 {
		etag := eTag(data)
		if etag != "" {
			// Check If-None-Match header
			if match := c.Get("If-None-Match"); match == etag {
				c.w.WriteHeader(http.StatusNotModified)
				return nil
			}
			c.Set("ETag", etag)
			c.Set("Cache-Control", "private, must-revalidate")
		}
		c.Set("ETag", etag)
	}

	c.w.Header().Set("Content-Type", "application/json")
	c.w.WriteHeader(status)
	return json.NewEncoder(c.w).Encode(data)
}

// String sends a plain text response
func (c *Context) String(status int, format string, values ...any) error {
	c.w.Header().Set("Content-Type", "text/plain")
	c.w.WriteHeader(status)
	_, err := fmt.Fprintf(c.w, format, values...)
	return err
}

func (c *Context) Method() string {
	return c.r.Method
}

func (c *Context) URLPath() string {
	return c.r.URL.Path
}

// Param gets a route parameter by key from Chi's URL params
func (c *Context) Param(key string) string {
	return chi.URLParam(c.r, key)
}

// Query gets a URL query parameter
func (c *Context) Query() url.Values {
	return c.r.URL.Query()
}

// Bind unmarshals JSON request body into provided struct
func (c *Context) Bind(dst any) error {
	return json.NewDecoder(c.r.Body).Decode(dst)
}

// Status sets the HTTP status code
func (c *Context) Status(code int) *Context {
	c.w.WriteHeader(code)
	return c
}

// Set sets a response header
func (c *Context) Set(key, value string) {
	c.w.Header().Set(key, value)
}

// Get gets a request header
func (c *Context) Get(key string) string {
	return c.r.Header.Get(key)
}

// SetCookie sets cookie
func (c *Context) SetCookie(cookie *http.Cookie) {
	http.SetCookie(c.w, cookie)
}

// GetCookie gets cookie value
func (c *Context) GetCookie(name string) (*http.Cookie, error) {
	return c.r.Cookie(name)
}

func (c *Context) Context() context.Context {
	return c.r.Context()
}

func (c *Context) WithContext(ctx context.Context) *Context {
	return &Context{
		r: c.r.WithContext(ctx),
		w: c.w,
	}
}

// FormValue gets a form value by key
func (c *Context) FormValue(key string) string {
	return c.r.FormValue(key)
}

// FormFile gets a single uploaded file
func (c *Context) FormFile(key string) (*multipart.FileHeader, error) {
	_, header, err := c.r.FormFile(key)
	if err != nil {
		return nil, err
	}

	return header, nil
}

func (c *Context) EnsureMultipartForm() error {
	if !strings.HasPrefix(c.Get("Content-Type"), "multipart/form-data") {
		return ErrMultipartForm.SetMessage("request 'Content-Type' header must be multipart/form-data")
	}
	return nil
}

func (c *Context) CleanupMultipart() {
	if c.r.MultipartForm != nil {
		_ = c.r.MultipartForm.RemoveAll()
	}
}

// FormFiles gets multiple uploaded files
func (c *Context) FormFiles(key string) ([]*multipart.FileHeader, error) {
	if c.r.MultipartForm == nil {
		if err := c.r.ParseMultipartForm(2 << 20); err != nil {
			return nil, err
		}
	}

	if c.r.MultipartForm != nil && c.r.MultipartForm.File != nil {
		if files := c.r.MultipartForm.File[key]; len(files) > 0 {
			return files, nil
		}
	}

	return nil, http.ErrMissingFile
}

// BindValidate binds a JSON request body into dst, then validates and/or sanitizes it.
// dst may implement [validator.Validator], [validator.Sanitizer], or both.
// If neither is implemented, [validator.Struct] is used as a fallback.
func (c *Context) BindValidate(dst any) error {
	if c.Get("Content-Type") != "application/json" {
		return &JSONBindError{Message: "Content-Type must be application/json"}
	}

	dec := json.NewDecoder(c.r.Body)
	if err := dec.Decode(dst); err != nil {
		return ParseJSONBindErr(err)
	}

	return validator.Validate(dst)
}

func (c *Context) Request() *http.Request {
	return c.r
}

func (c *Context) Writer() http.ResponseWriter {
	return c.w
}

type ErrorBody struct {
	Error *Error `json:"error"`
}

// Handler converts custom HandlerFunc to standard http.HandlerFunc with structured error response handling capability
func Handler(h Func) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := &Context{w: w, r: r}
		if err := h(c); err != nil {
			e := NormalizeError(err)
			if err := c.JSON(e.Status(), &ErrorBody{Error: e}); err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}
	}
}

// Wrap converts standard http.HandlerFunc to custom Func
func Wrap(h http.HandlerFunc) Func {
	return func(c *Context) error {
		h(c.w, c.r)
		return nil
	}
}

// WrapHandler converts http.Handler to custom Func
func WrapHandler(h http.Handler) Func {
	return func(c *Context) error {
		h.ServeHTTP(c.w, c.r)
		return nil
	}
}

// Middleware converts Func middleware into std middleware signature
func Middleware(mw func(Func) Func) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return Handler(mw(WrapHandler(next)))
	}
}
