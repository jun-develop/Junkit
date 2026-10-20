package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/jun-develop/junkit/notification"
	"github.com/jun-develop/junkit/response"
	"github.com/jun-develop/junkit/tracer"
)

// RecoverOptions configures panic recovery behavior and debug bypass settings.
type RecoverOptions struct {
	// Debug selects requests whose panic goes to the tracer instead of the 500 response. Use the
	// same options as TracerDebug.
	Debug TracerDebugOptions
}

// Recover creates a Gin middleware that catches panics, logs stack traces, sends error
// notifications and returns HTTP 500. tracer.BreakpointSignal panics are re-panicked.
//
// Usage:
//
//	router.Use(middleware.Recover(zapLogger, notifier, responder, middleware.RecoverOptions{}))
func Recover(logger *zap.Logger, notifier notification.Notifier, responder response.Responder, opts RecoverOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			r := recover()
			if r == nil {
				return
			}

			if _, ok := r.(tracer.BreakpointSignal); ok {
				panic(r)
			}

			if opts.Debug.Matches(c) {
				tracer.E(c.Request.Context(), "PANIC RECOVERED", r)
				return
			}

			handlePanic(c, logger, notifier, responder, r)
		}()

		c.Next()
	}
}

// handlePanic logs and reports a recovered panic, then aborts with a 500 response.
func handlePanic(c *gin.Context, logger *zap.Logger, notifier notification.Notifier, responder response.Responder, r any) {
	file, line, fn := tracer.FindErrorOrigin()
	stack := debug.Stack()

	logger.Error("middleware: panic recovered",
		zap.Any("error", r),
		zap.String("location", file),
		zap.Int("line", line),
		zap.String("function", fn),
		zap.ByteString("stack", stack),
	)

	errorData := map[string]any{
		"error":     fmt.Sprintf("%v", r),
		"location":  file,
		"line":      line,
		"function":  fn,
		"stack":     string(stack),
		"timestamp": time.Now().Format(time.RFC3339Nano),
	}
	if notifier != nil {
		notifier.SendErrorNotification(c, errorData, http.StatusInternalServerError)
	}

	responder.Error(c, response.ErrInternalServer)
	c.Abort()
}
