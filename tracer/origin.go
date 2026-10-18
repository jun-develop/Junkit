package tracer

import (
	"runtime"
	"strings"
)

// tracerPackagePrefix identifies call frames belonging to this package.
const tracerPackagePrefix = "github.com/jun-develop/junkit/tracer."

// FindErrorOrigin returns the file, line and function of the first caller outside this package
// and the Go runtime, i.e. the application code that triggered an error.
func FindErrorOrigin() (string, int, string) {
	pc := make([]uintptr, 32)
	// skip=3 skips runtime.Callers, FindErrorOrigin, and the calling log helper.
	n := runtime.Callers(3, pc)
	frames := runtime.CallersFrames(pc[:n])

	for {
		frame, more := frames.Next()
		if !strings.Contains(frame.File, "/runtime/") &&
			!strings.Contains(frame.File, "/src/runtime/") &&
			!strings.HasPrefix(frame.Function, tracerPackagePrefix) {
			return frame.File, frame.Line, frame.Function
		}

		if !more {
			break
		}
	}

	return "unknown", 0, "unknown"
}
