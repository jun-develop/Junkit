package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/jun-develop/junkit/middleware"
)

func TestRoutePatterns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RoutePatterns(map[string]func(string) bool{
		"uuid": middleware.IsUUID,
		"id":   middleware.IsID,
		"slug": middleware.IsSlug,
	}, newResponder()))

	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	router.GET("/users/:uuid", ok)
	router.GET("/orders/:id", ok)
	router.GET("/posts/:slug/comments/:id", ok)
	router.GET("/files/:key", ok)

	tests := []struct {
		path string
		want int
	}{
		{"/users/9b2f6c1e-3a4d-4f5e-8a7b-1c2d3e4f5a6b", http.StatusOK},
		{"/users/abc", http.StatusNotFound},
		{"/users/9b2f6c1e3a4d4f5e8a7b1c2d3e4f5a6b", http.StatusNotFound},
		{"/orders/42", http.StatusOK},
		{"/orders/0", http.StatusNotFound},
		{"/orders/-1", http.StatusNotFound},
		{"/orders/99999999999999999999", http.StatusNotFound},
		{"/orders/12abc", http.StatusNotFound},
		{"/posts/hello-world-2/comments/7", http.StatusOK},
		{"/posts/Hello_World/comments/7", http.StatusNotFound},
		{"/posts/hello--world/comments/7", http.StatusNotFound},
		{"/posts/hello/comments/x", http.StatusNotFound},
		{"/files/anything_GOES.txt", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d", w.Code, tt.want)
			}
		})
	}
}
