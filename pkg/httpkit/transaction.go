package httpkit

import (
	"bytes"
	"cmp"
	"context"
	"maps"
	"net/http"

	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/port/comproto"
)

// TransactionMiddleware runs each state-changing request (POST, PUT, PATCH, DELETE)
// in a transaction of the CommitManager.
// The handlers down the line join the transaction through the request context.
//
// The transaction is committed when the reply has a success status code,
// and rolled back when the reply has an error status code (400 or above),
// or when the request panics, in which case the panic is passed on.
// The reply is held back until the commit, so a failed commit replies with an error rather than a success.
//
// Safe methods (GET, HEAD, OPTIONS, TRACE) don't change state.
// They run without a transaction, and their reply streams to the client as it's written,
// so a long list reply isn't held in memory.
type TransactionMiddleware struct {
	Next          http.Handler
	CommitManager comproto.OnePhaseCommitProtocol
	// ErrorHandler [optional] replies when the transaction can't begin or can't be committed.
	//
	// default: rfc7807.Handler{Mapping: ErrorMapping}
	ErrorHandler ErrorHandler
}

func (h TransactionMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if isSafeMethod(r.Method) {
		h.Next.ServeHTTP(w, r)
		return
	}

	ctx, err := h.CommitManager.BeginTx(r.Context())
	if err != nil {
		logger.Error(r.Context(), "httpkit.TransactionMiddleware failed to begin a transaction", logging.ErrField(err))
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	var finished bool
	defer func() {
		if !finished { // Next panicked, and the panic carries on after the rollback.
			h.rollback(r, ctx)
		}
	}()

	resp := &responseBuffer{header: make(http.Header)}
	h.Next.ServeHTTP(resp, r.WithContext(ctx))
	finished = true

	if resp.statusCode() >= http.StatusBadRequest {
		h.rollback(r, ctx)
	} else if err := h.CommitManager.CommitTx(ctx); err != nil {
		logger.Error(r.Context(), "httpkit.TransactionMiddleware failed to commit the transaction", logging.ErrField(err))
		h.getErrorHandler().HandleError(w, r, err)
		return
	}

	if err := resp.writeTo(w); err != nil {
		logger.Debug(r.Context(), "error during writing response to the caller", logging.ErrField(err))
	}
}

func (h TransactionMiddleware) rollback(r *http.Request, tx context.Context) {
	if err := h.CommitManager.RollbackTx(tx); err != nil {
		logger.Error(r.Context(), "httpkit.TransactionMiddleware failed to roll back the transaction", logging.ErrField(err))
	}
}

func (h TransactionMiddleware) getErrorHandler() ErrorHandler {
	if h.ErrorHandler != nil {
		return h.ErrorHandler
	}
	return defaultErrorHandler
}

// isSafeMethod reports whether the request method is read-only, as defined in RFC 9110 §9.2.1.
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

// responseBuffer holds back a reply, so it can be replaced when it turns out to be wrong.
type responseBuffer struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (b *responseBuffer) Header() http.Header { return b.header }

func (b *responseBuffer) WriteHeader(code int) {
	if b.code == 0 {
		b.code = code
	}
}

func (b *responseBuffer) Write(p []byte) (int, error) {
	b.WriteHeader(http.StatusOK)
	return b.body.Write(p)
}

func (b *responseBuffer) statusCode() int {
	return cmp.Or(b.code, http.StatusOK)
}

func (b *responseBuffer) writeTo(w http.ResponseWriter) error {
	maps.Copy(w.Header(), b.header)
	w.WriteHeader(b.statusCode())
	_, err := b.body.WriteTo(w)
	return err
}
