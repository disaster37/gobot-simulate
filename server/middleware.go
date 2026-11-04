package server

import (
	"context"
	"crypto/subtle"
	"log"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

const (
	contentSecurityPolicy = "default-src 'self'; script-src 'self' 'unsafe-inline'; " +
		"style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; " +
		"connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'self'"
)

func (s *Server) recoveryMiddleware(ctx context.Context, c *app.RequestContext) {
	defer func() {
		if recovered := recover(); recovered != nil {
			// Log the panic and stack server-side; never echo the raw panic
			// value to the client (it may contain internal paths or data).
			log.Printf("panic recovered while serving %s %s: %v\n%s",
				c.Method(), c.Path(), recovered, debug.Stack())
			writeError(c, "", http.StatusInternalServerError, "INTERNAL", "internal server error")
			c.Abort()
		}
	}()
	c.Next(ctx)
}

func (s *Server) securityHeaders(ctx context.Context, c *app.RequestContext) {
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "SAMEORIGIN")
	c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
	c.Header("Content-Security-Policy", contentSecurityPolicy)
	c.Next(ctx)
}

func (s *Server) corsMiddleware(ctx context.Context, c *app.RequestContext) {
	origin := string(c.GetHeader("Origin"))
	if origin == "" {
		c.Next(ctx)
		return
	}

	allowed := s.originAllowed(origin)
	if allowed {
		if s.allowAllOrigins() {
			c.Header("Access-Control-Allow-Origin", "*")
		} else {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, Last-Event-ID")
		c.Header("Access-Control-Expose-Headers", "Last-Event-ID")
		c.Header("Access-Control-Max-Age", "600")
	}

	if string(c.Method()) == http.MethodOptions {
		if !allowed {
			writeError(c, "", http.StatusForbidden, "FORBIDDEN", "origin is not allowed")
			c.Abort()
			return
		}
		c.AbortWithStatus(http.StatusNoContent)
		return
	}
	c.Next(ctx)
}

func (s *Server) authMiddleware(ctx context.Context, c *app.RequestContext) {
	if s.cfg.AuthToken == "" || isHealthPath(string(c.Path())) || string(c.Method()) == http.MethodOptions {
		c.Next(ctx)
		return
	}

	path := string(c.Path())
	if !strings.HasPrefix(path, "/api/") && path != "/api" {
		c.Next(ctx)
		return
	}

	token := bearerToken(string(c.GetHeader("Authorization")))
	if token == "" && path == "/api/v1/events" {
		token = c.Query("token")
	}
	if constantTimeEqual(token, s.cfg.AuthToken) {
		c.Next(ctx)
		return
	}
	writeError(c, "", http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid bearer token")
	c.Abort()
}

func (s *Server) bodyLimitMiddleware(ctx context.Context, c *app.RequestContext) {
	raw := string(c.GetHeader("Content-Length"))
	if raw != "" {
		size, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			writeError(c, "", http.StatusBadRequest, "INVALID_CONTENT_LENGTH", "invalid Content-Length header")
			c.Abort()
			return
		}
		if size > maxRequestBodySize {
			writeError(c, "", http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body exceeds 64 KB")
			c.Abort()
			return
		}
	}
	c.Next(ctx)
}

func (s *Server) originAllowed(origin string) bool {
	for _, allowed := range s.cfg.CORSOrigins {
		if allowed == "*" || allowed == origin {
			return true
		}
	}
	return false
}

func (s *Server) allowAllOrigins() bool {
	for _, allowed := range s.cfg.CORSOrigins {
		if allowed == "*" {
			return true
		}
	}
	return false
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return header[len(prefix):]
}

func constantTimeEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func isHealthPath(path string) bool {
	return path == "/api/v1/health"
}
