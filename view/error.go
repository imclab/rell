package view

import (
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/daaku/go.errcode"
	"github.com/daaku/go.h"
	"github.com/daaku/go.static"
)

// HTTP Coded Error.
type ErrorCode interface {
	error
	Code() int
}

// http.Handler for ErrorCode.
type errorCodeHandler struct {
	Static *static.Handler
	err    ErrorCode
}

// Serve an appropriate response for this error. Currently this means
// HTML or Plain Text.
// TODO(naitik): Extend for JSON.
func (err errorCodeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	code := err.err.Code()
	if code == 0 {
		code = http.StatusInternalServerError
	}
	if usePlainText(r) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(code)
		io.Copy(w, strings.NewReader(err.err.Error()))
		w.Write([]byte("\n"))
	} else {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(code)
		page := &Page{
			Static: err.Static,
			Body:   h.String(err.err.Error()),
		}
		h.WriteResponse(w, r, page)
	}
}

// Send a error response. If the error also implements http.Handler,
// it will simply be passed control, otherwise the default error
// rendering will be used.
func Error(w http.ResponseWriter, r *http.Request, s *static.Handler, err error) {
	handler, ok := err.(http.Handler)
	if !ok {
		errCode, ok := err.(ErrorCode)
		if !ok {
			errCode = errcode.Add(500, err)
			log.Printf("Error %d: %s %s %v", errCode.Code(), r.URL, err, err)
		}
		handler = errorCodeHandler{
			Static: s,
			err:    errCode,
		}
	}
	handler.ServeHTTP(w, r)
}

func usePlainText(r *http.Request) bool {
	return strings.Contains(r.UserAgent(), "curl")
}
