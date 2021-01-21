package handler

import (
	"context"

	"github.com/sourcegraph/jsonrpc2"
)

// AsyncHandler wraps a Handler such that each request is handled in its own goroutine.
type AsyncHandler struct {
	handler jsonrpc2.Handler
}

// Handle handles a request or notification
func (ah AsyncHandler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	go ah.handler.Handle(ctx, conn, req)
}
