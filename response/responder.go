package response

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jun-develop/junkit/i18n"
	"github.com/jun-develop/junkit/pagination"
	"github.com/jun-develop/junkit/response/filter"
)

// Options configures a Responder.
type Options struct {
	// Language is the default locale for i18n messages (e.g. "en").
	Language string
}

// Responder writes localized JSON responses in the Response envelope; key is an i18n message
// key formatted with args. Send writes a message-only response. SendWithData puts data in "data"
// for status < 400 and in "error" otherwise. Pagination wraps data and its metadata in
// PaginatedData. Successful SendWithData and Pagination responses honour "?fields="/"?omit="
// filtering. Translate resolves a key to a localized string. Error maps an *AppError to its
// Code's HTTP status and translated Message; any other error is recorded on the gin context and
// answered with a generic 500.
//
// Usage:
//
//	responder.SendWithData(ctx, http.StatusOK, "user_fetched", user)
//	responder.Error(ctx, err)
type Responder interface {
	Send(ctx *gin.Context, statusCode int, key string, args ...any)
	SendWithData(ctx *gin.Context, statusCode int, key string, data any, args ...any)
	Pagination(ctx *gin.Context, statusCode int, key string, data any, pagination pagination.Pagination, args ...any)
	Translate(key string, args ...any) string
	Error(ctx *gin.Context, err error)
}

// responder is the default Responder implementation.
type responder struct {
	translator *i18n.Translator
	lang       string
}

// NewResponder creates a Responder with the given Options and i18n Translator.
//
// Usage:
//
//	responder := response.NewResponder(response.Options{Language: "en"}, translator)
func NewResponder(opts Options, translator *i18n.Translator) Responder {
	return &responder{
		translator: translator,
		lang:       opts.Language,
	}
}

// Send implements Responder.
func (r *responder) Send(ctx *gin.Context, statusCode int, key string, args ...any) {
	message := r.translator.GetMessage(key, r.lang, args...)

	ctx.JSON(statusCode, Response{
		Status:  statusText(statusCode),
		Code:    httpStatusToErrorCode(statusCode),
		Message: message,
		APIInfo: getAPIInfo(ctx),
	})
}

// SendWithData implements Responder.
func (r *responder) SendWithData(ctx *gin.Context, statusCode int, key string, data any, args ...any) {
	message := r.translator.GetMessage(key, r.lang, args...)

	res := Response{
		Status:  statusText(statusCode),
		Code:    httpStatusToErrorCode(statusCode),
		Message: message,
		APIInfo: getAPIInfo(ctx),
	}

	if statusCode >= http.StatusBadRequest {
		res.Error = data
	} else {
		res.Data = data
	}

	r.renderJSON(ctx, statusCode, res)
}

// Pagination implements Responder.
func (r *responder) Pagination(ctx *gin.Context, statusCode int, key string, data any, pagination pagination.Pagination, args ...any) {
	message := r.translator.GetMessage(key, r.lang, args...)

	res := Response{
		Status:  statusText(statusCode),
		Message: message,
		Data: PaginatedData{
			Items:      data,
			Pagination: pagination,
		},
		APIInfo: getAPIInfo(ctx),
	}

	r.renderJSON(ctx, statusCode, res)
}

// Translate implements Responder.
func (r *responder) Translate(key string, args ...any) string {
	return r.translator.GetMessage(key, r.lang, args...)
}

// Error implements Responder.
func (r *responder) Error(ctx *gin.Context, err error) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		statusCode := errorCodeToHTTPStatus(appErr.Code)
		message := r.translator.GetMessage(appErr.Message, r.lang)

		ctx.JSON(statusCode, Response{
			Status:  "error",
			Code:    appErr.Code,
			Message: message,
			APIInfo: getAPIInfo(ctx),
		})

		return
	}

	// gin's ctx.Error panics on nil; a nil err still gets the generic 500.
	if err != nil {
		_ = ctx.Error(err)
	}

	r.Send(ctx, http.StatusInternalServerError, "internal_server_error")
}

// renderJSON writes res, applying "?fields="/"?omit=" filtering to successful responses; if
// filtering fails the unfiltered response is sent.
func (r *responder) renderJSON(ctx *gin.Context, statusCode int, res Response) {
	if statusCode < http.StatusBadRequest {
		fields := ctx.GetStringSlice("_response_fields")
		omit := ctx.GetStringSlice("_response_omit")

		if len(fields) > 0 || len(omit) > 0 {
			if filtered, err := filter.Filter(res, fields, omit); err == nil {
				ctx.JSON(statusCode, filtered)
				return
			}
		}
	}

	ctx.JSON(statusCode, res)
}

// errorCodeToHTTPStatus maps ErrorCode to HTTP status, defaulting to 500.
func errorCodeToHTTPStatus(code ErrorCode) int {
	switch code {
	case BadRequest:
		return http.StatusBadRequest
	case Unauthorized:
		return http.StatusUnauthorized
	case NotFound:
		return http.StatusNotFound
	case Forbidden:
		return http.StatusForbidden
	case MethodNotAllowed:
		return http.StatusMethodNotAllowed
	case Conflict:
		return http.StatusConflict
	case UnsupportedMediaType:
		return http.StatusUnsupportedMediaType
	case PayloadTooLarge:
		return http.StatusRequestEntityTooLarge
	case TooManyRequests:
		return http.StatusTooManyRequests
	case UnprocessableEntity:
		return http.StatusUnprocessableEntity
	case Internal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// httpStatusToErrorCode gives every error status (>= 400) a code so error bodies always carry one;
// unmapped statuses fall back to BAD_REQUEST (4xx) or INTERNAL_SERVER_ERROR (5xx).
func httpStatusToErrorCode(status int) ErrorCode {
	switch {
	case status < http.StatusBadRequest:
		return ""
	case status == http.StatusBadRequest:
		return BadRequest
	case status == http.StatusUnauthorized:
		return Unauthorized
	case status == http.StatusForbidden:
		return Forbidden
	case status == http.StatusNotFound:
		return NotFound
	case status == http.StatusMethodNotAllowed:
		return MethodNotAllowed
	case status == http.StatusConflict:
		return Conflict
	case status == http.StatusRequestEntityTooLarge:
		return PayloadTooLarge
	case status == http.StatusUnsupportedMediaType:
		return UnsupportedMediaType
	case status == http.StatusUnprocessableEntity:
		return UnprocessableEntity
	case status == http.StatusTooManyRequests:
		return TooManyRequests
	case status < http.StatusInternalServerError:
		return BadRequest
	default:
		return Internal
	}
}

// statusText returns "error" for status >= 400, "success" otherwise.
func statusText(code int) string {
	if code >= http.StatusBadRequest {
		return "error"
	}

	return "success"
}
