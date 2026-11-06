package server

import (
	"context"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

func (s *Server) handleNoRoute(ctx context.Context, c *app.RequestContext) {
	if !s.cfg.UIEnabled || s.cfg.Assets == nil {
		writeError(c, "", http.StatusNotFound, "NOT_FOUND", "resource not found")
		return
	}
	s.serveStatic(ctx, c)
}

func (s *Server) serveStatic(_ context.Context, c *app.RequestContext) {
	requestPath := string(c.Path())
	if isAPIPath(requestPath) {
		writeError(c, "", http.StatusNotFound, "NOT_FOUND", "api route not found")
		return
	}
	originalPath := string(c.Request.URI().PathOriginal())
	lowerOriginal := strings.ToLower(originalPath)
	if strings.Contains(requestPath, "..") || strings.Contains(originalPath, "..") ||
		strings.Contains(lowerOriginal, "%2e%2e") || strings.ContainsRune(requestPath, '\x00') ||
		strings.Contains(lowerOriginal, "%00") {
		writeError(c, "", http.StatusBadRequest, "INVALID_PATH", "invalid path")
		return
	}

	cleaned := path.Clean("/" + requestPath)
	relative := strings.TrimPrefix(cleaned, "/")
	if relative == "" || relative == "." {
		s.writeIndex(c)
		return
	}

	if info, err := fs.Stat(s.cfg.Assets, relative); err == nil && !info.IsDir() {
		s.writeAsset(c, relative)
		return
	}
	// SPA history-mode fallback for client routes such as /boards/dfp.
	s.writeIndex(c)
}

func (s *Server) writeIndex(c *app.RequestContext) {
	data, err := fs.ReadFile(s.cfg.Assets, "index.html")
	if err != nil {
		writeError(c, "", http.StatusNotFound, "NOT_FOUND", "web UI is not embedded")
		return
	}
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

func (s *Server) writeAsset(c *app.RequestContext, relative string) {
	data, err := fs.ReadFile(s.cfg.Assets, relative)
	if err != nil {
		s.writeIndex(c)
		return
	}
	contentType := mime.TypeByExtension(path.Ext(relative))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	if strings.HasPrefix(relative, "_nuxt/") {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		c.Header("Cache-Control", "no-cache")
	}
	c.Data(http.StatusOK, contentType, data)
}

func isAPIPath(requestPath string) bool {
	return requestPath == "/api" || strings.HasPrefix(requestPath, "/api/")
}
