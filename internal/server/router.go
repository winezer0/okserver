package server

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/winezer0/okserver/internal/webui"
)

type transportWriterKey struct{}
type transportWriter struct {
	gin.ResponseWriter
	original http.ResponseWriter
}

func (writer *transportWriter) Unwrap() http.ResponseWriter { return writer.original }
func init()                                                 { gin.SetMode(gin.ReleaseMode) }

func (fs *fileServer) endpoint(next http.HandlerFunc, needsUpload bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		original := c.Request.Context().Value(transportWriterKey{}).(http.ResponseWriter)
		writer := &transportWriter{ResponseWriter: c.Writer, original: original}
		fs.middleware(writer, c.Request, func(w http.ResponseWriter, r *http.Request) {
			if needsUpload && fs.uploads == nil {
				writeAPIError(w, 503, "upload service is unavailable", nil)
				return
			}
			next(w, r)
		})
	}
}

func (fs *fileServer) routes() http.Handler {
	router := gin.New()
	router.RedirectTrailingSlash = false
	router.RedirectFixedPath = false
	router.HandleMethodNotAllowed = true
	_ = router.SetTrustedProxies(nil)
	bind := func(method, path string, handler http.HandlerFunc) {
		router.Handle(method, path, fs.endpoint(handler, false))
	}
	for _, method := range []string{"POST", "PUT", "HEAD", "GET", "DELETE"} {
		router.Handle(method, "/upload/*path", fs.uploadEndpoint(fs.directUpload, method == "POST" || method == "PUT"))
	}
	bind("GET", "/download/*path", fs.directDownload)
	bind("HEAD", "/download/*path", fs.directDownload)
	{
		const prefix = "/api/v1"
		bind("GET", prefix+"/files", fs.handleFileList)
		for _, route := range []struct {
			method, path string
			handler      http.HandlerFunc
			write        bool
		}{
			{"GET", "/uploads", fs.handleUploadList, false},
			{"POST", "/uploads/cleanup", fs.handleUploadCleanup, false},
			{"GET", "/events", fs.handleEvents, false},
		} {
			router.Handle(route.method, prefix+route.path, fs.uploadEndpoint(route.handler, route.write))
		}
		for _, route := range []struct {
			method, path string
			handler      func(http.ResponseWriter, *http.Request, string)
			write        bool
		}{
			{"GET", "/uploads/:id", fs.handleUploadGet, false},
			{"DELETE", "/uploads/:id", fs.handleUploadCancel, false},
		} {
			router.Handle(route.method, prefix+route.path, func(c *gin.Context) {
				fs.uploadEndpoint(func(w http.ResponseWriter, r *http.Request) { route.handler(w, r, c.Param("id")) }, route.write)(c)
			})
		}

	}
	for _, path := range []string{"/healthz", "/readyz"} {
		bind("GET", path, fs.handleHealth)
		bind("HEAD", path, fs.handleHealth)
	}
	bind("GET", "/metrics", fs.metrics.ServeHTTP)
	bind("HEAD", "/metrics", fs.metrics.ServeHTTP)
	bind("GET", "/", fs.handleIndex)
	bind("HEAD", "/", fs.handleIndex)
	for _, path := range []string{"/workspace", "/workspace/*path"} {
		bind("GET", path, webui.Index)
		bind("HEAD", path, webui.Index)
	}
	router.NoMethod(fs.endpoint(func(w http.ResponseWriter, r *http.Request) { writeAPIError(w, 405, "method not allowed", nil) }, false))
	router.NoRoute(fs.endpoint(func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(w, http.StatusNotFound, "endpoint not found", nil)
	}, false))

	return router
}

func (fs *fileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fs.routerOnce.Do(func() { fs.router = fs.routes() })
	r = r.WithContext(context.WithValue(r.Context(), transportWriterKey{}, w))
	fs.router.ServeHTTP(w, r)
}
