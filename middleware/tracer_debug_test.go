package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/jun-develop/junkit/middleware"
	"github.com/jun-develop/junkit/tracer"
)

var debugOpts = middleware.TracerDebugOptions{
	QueryKey:     "t_debug",
	QueryValue:   "1234",
	SecretHeader: "X-Internal-Secret",
	Secret:       "s3cret",
}

// newDebugRequest builds a GET for target carrying debugOpts' secret header.
func newDebugRequest(target string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set(debugOpts.SecretHeader, debugOpts.Secret)

	return req
}

func newTracerDebugRouter(opts middleware.TracerDebugOptions, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.TracerDebug(opts))
	router.GET("/", handler)

	return router
}

func TestTracerDebug_PassesThroughWhenNotActivated(t *testing.T) {
	router := newTracerDebugRouter(debugOpts, func(c *gin.Context) {
		c.String(http.StatusTeapot, "normal response")
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want %d (the handler's own response, untouched)", w.Code, http.StatusTeapot)
	}

	if w.Body.String() != "normal response" {
		t.Fatalf("body = %q, want the handler's own response", w.Body.String())
	}
}

func TestTracerDebug_RequiresQueryAndSecret(t *testing.T) {
	emptySecret := debugOpts
	emptySecret.Secret = ""

	wrongSecret := httptest.NewRequest(http.MethodGet, "/?t_debug=1234", nil)
	wrongSecret.Header.Set(debugOpts.SecretHeader, "wrong")

	tests := []struct {
		name string
		opts middleware.TracerDebugOptions
		req  *http.Request
	}{
		{"empty options", middleware.TracerDebugOptions{}, newDebugRequest("/?t_debug=")},
		{"empty secret", emptySecret, httptest.NewRequest(http.MethodGet, "/?t_debug=1234", nil)},
		{"missing header", debugOpts, httptest.NewRequest(http.MethodGet, "/?t_debug=1234", nil)},
		{"wrong secret", debugOpts, wrongSecret},
		{"wrong query value", debugOpts, newDebugRequest("/?t_debug=0000")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newTracerDebugRouter(tt.opts, func(c *gin.Context) {
				c.String(http.StatusOK, "normal")
			})

			w := httptest.NewRecorder()
			router.ServeHTTP(w, tt.req)

			if w.Body.String() != "normal" {
				t.Fatalf("body = %q, want the handler's own response", w.Body.String())
			}
		})
	}
}

func TestTracerDebug_RendersDashboardWhenActivated(t *testing.T) {
	router := newTracerDebugRouter(debugOpts, func(c *gin.Context) {
		c.String(http.StatusTeapot, "normal response")
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, newDebugRequest("/?t_debug=1234"))

	// The handler's output is discarded; the dashboard always renders 200 with its own body.
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d for the debug dashboard", w.Code, http.StatusOK)
	}

	if strings.Contains(w.Body.String(), "normal response") {
		t.Fatal("the handler's real response body leaked through instead of being replaced by the dashboard")
	}

	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html for a browser client", ct)
	}
}

func TestTracerDebug_RendersJSONForCLIClients(t *testing.T) {
	router := newTracerDebugRouter(debugOpts, func(c *gin.Context) {
		c.String(http.StatusOK, "normal response")
	})

	req := newDebugRequest("/?t_debug=1234")
	req.Header.Set("User-Agent", "curl/8.0.1")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Body.Len() == 0 {
		t.Fatal("expected a non-empty rendered body for a curl client")
	}

	// A curl client gets the JSON report (pipeable into jq), not HTML markup.
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json for a CLI client", ct)
	}

	if !json.Valid(w.Body.Bytes()) {
		t.Errorf("body is not valid JSON: %s", w.Body.String())
	}
}

func TestTracerDebug_BreakpointSignalRendersDashboardInstead(t *testing.T) {
	router := newTracerDebugRouter(debugOpts, func(c *gin.Context) {
		panic(tracer.BreakpointSignal{})
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, newDebugRequest("/?t_debug=1234"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d — a BreakpointSignal should still render the dashboard", w.Code, http.StatusOK)
	}

	if w.Body.Len() == 0 {
		t.Fatal("expected a rendered dashboard body after a BreakpointSignal")
	}
}

func TestTracerDebug_OtherPanicsPropagate(t *testing.T) {
	// Non-breakpoint panics must propagate so Recover can handle them.
	router := newTracerDebugRouter(debugOpts, func(c *gin.Context) {
		panic("a real error, not a breakpoint")
	})

	defer func() {
		r := recover()
		if r != "a real error, not a breakpoint" {
			t.Fatalf("recover() = %v, want the original panic value to propagate unchanged", r)
		}
	}()

	router.ServeHTTP(httptest.NewRecorder(), newDebugRequest("/?t_debug=1234"))
}

func TestTracerDebug_DropsInnerGzipEncoding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.TracerDebug(debugOpts), middleware.Gzip(middleware.GzipOptions{}))
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "normal response")
	})

	req := newDebugRequest("/?t_debug=1234")
	req.Header.Set("Accept-Encoding", "gzip")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// The dashboard is written uncompressed, so a leftover gzip header breaks every client.
	if ce := w.Header().Get("Content-Encoding"); ce != "" {
		t.Fatalf("Content-Encoding = %q, want none for the plain dashboard", ce)
	}

	if !strings.HasPrefix(w.Body.String(), "<!DOCTYPE html>") {
		t.Fatalf("body = %.40q, want the plain HTML dashboard", w.Body.String())
	}
}

func TestTracerDebug_StaysOKWhenInnerMiddlewareAborts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.TracerDebug(debugOpts), middleware.CORS(middleware.CORSOptions{}))
	router.OPTIONS("/", func(c *gin.Context) {})

	req := newDebugRequest("/?t_debug=1234")
	req.Method = http.MethodOptions
	req.Header.Set("User-Agent", "curl/8")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !json.Valid(w.Body.Bytes()) {
		t.Fatalf("status = %d body = %q, want 200 with the JSON dashboard (CORS's 204 discarded)", w.Code, w.Body)
	}
}
