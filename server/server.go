package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

type Config struct {
	// Feature Flags
	EnableHTTP bool `envconfig:"ENABLE_HTTP" default:"true"`
	EnableGRPC bool `envconfig:"ENABLE_GRPC" default:"false"`

	// HTTP Config
	HTTPPort         string        `envconfig:"HTTP_PORT" default:"8080"`
	HTTPReadTimeout  time.Duration `envconfig:"HTTP_READ_TIMEOUT" default:"10s"`
	HTTPWriteTimeout time.Duration `envconfig:"HTTP_WRITE_TIMEOUT" default:"10s"`

	// gRPC Config
	GRPCPort string `envconfig:"GRPC_PORT" default:"9090"`

	// Common
	ShutdownTimeout time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"10s"`
}

type Server struct {
	cfg        Config
	log        *slog.Logger
	httpServer *http.Server
	grpcServer *grpc.Server
}

// New initializes the server wrapper.
// Pass nil for grpcServer if EnableGRPC is false, or pass a configured *grpc.Server.
func New(cfg Config, logger *slog.Logger, httpHandler http.Handler, grpcServer *grpc.Server) *Server {
	s := &Server{
		cfg: cfg,
		log: logger,
	}

	if cfg.EnableHTTP {
		s.httpServer = &http.Server{
			Addr:         ":" + cfg.HTTPPort,
			Handler:      httpHandler,
			ReadTimeout:  cfg.HTTPReadTimeout,
			WriteTimeout: cfg.HTTPWriteTimeout,
		}
	}

	if cfg.EnableGRPC {
		if grpcServer == nil {
			// Fallback: Create a default one if none provided but enabled
			// Ideally, the caller should register services before passing it here.
			s.grpcServer = grpc.NewServer()
		} else {
			s.grpcServer = grpcServer
		}
	}

	return s
}

func (s *Server) Start(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	// 1. Start HTTP
	if s.cfg.EnableHTTP && s.httpServer != nil {
		g.Go(func() error {
			s.log.Info("Starting HTTP server", "port", s.cfg.HTTPPort)
			if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				return fmt.Errorf("http server failed: %w", err)
			}
			return nil
		})
	}

	// 2. Start gRPC
	if s.cfg.EnableGRPC && s.grpcServer != nil {
		g.Go(func() error {
			lis, err := net.Listen("tcp", ":"+s.cfg.GRPCPort)
			if err != nil {
				return fmt.Errorf("failed to listen for grpc: %w", err)
			}
			s.log.Info("Starting gRPC server", "port", s.cfg.GRPCPort)
			if err := s.grpcServer.Serve(lis); err != nil {
				return fmt.Errorf("grpc server failed: %w", err)
			}
			return nil
		})
	}

	// 3. Wait for Shutdown Signal
	g.Go(func() error {
		<-ctx.Done()
		s.log.Info("Shutdown signal received")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
		defer cancel()

		var shutdownErr error

		if s.httpServer != nil {
			s.log.Info("Shutting down HTTP server...")
			if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
				s.log.Error("HTTP shutdown error", "error", err)
				shutdownErr = err
			}
		}

		if s.grpcServer != nil {
			s.log.Info("Shutting down gRPC server...")
			s.grpcServer.GracefulStop()
		}

		return shutdownErr
	})

	return g.Wait()
}
