package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPCRecoveryInterceptor handles panics in gRPC unary calls.
// It ensures the process does not exit on handler panics.
func GRPCRecoveryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			// Capture Stack
			stack := string(debug.Stack())

			// Log with Context
			slog.ErrorContext(ctx, "GRPC PANIC RECOVERED",
				"error", fmt.Sprintf("%v", rec),
				"method", info.FullMethod,
				"stack", stack,
			)

			// Return Internal Error to client
			err = status.Errorf(codes.Internal, "internal server error")
		}
	}()

	return handler(ctx, req)
}
