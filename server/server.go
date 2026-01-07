package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"google.golang.org/grpc"
)

type Config struct {
	EnableHTTP       bool
	EnableGRPC       bool
	HTTPPort         string
	GRPCPort         string
	HTTPReadTimeout  time.Duration
	HTTPWriteTimeout time.Duration
	ShutdownTimeout  time.Duration

	// mTLS Configuration
	MTLSEnabled    bool
	MTLSCACert     string // Path to CA Certificate
	MTLSServerCert string // Path to Server Certificate
	MTLSServerKey  string // Path to Server Key
}

type Server struct {
	cfg     Config
	logger  *slog.Logger
	router  *chi.Mux
	grpcSrv *grpc.Server
	httpSrv *http.Server
}

func New(cfg Config, logger *slog.Logger, router *chi.Mux, grpcSrv *grpc.Server) *Server {
	return &Server{
		cfg:     cfg,
		logger:  logger,
		router:  router,
		grpcSrv: grpcSrv,
	}
}

func (s *Server) Start(ctx context.Context) error {
	errChan := make(chan error, 2)

	// Start HTTP Server
	if s.cfg.EnableHTTP {
		s.httpSrv = &http.Server{
			Addr:              ":" + s.cfg.HTTPPort,
			Handler:           s.router,
			ReadTimeout:       s.cfg.HTTPReadTimeout,
			ReadHeaderTimeout: 5 * time.Second,
			WriteTimeout:      s.cfg.HTTPWriteTimeout,
			IdleTimeout:       120 * time.Second,
		}

		if s.cfg.MTLSEnabled {
			s.logger.Info("Enabling mTLS for HTTP Server")
			tlsConfig, err := loadMTLSConfig(s.cfg.MTLSCACert)
			if err != nil {
				return fmt.Errorf("failed to load mTLS config: %w", err)
			}
			s.httpSrv.TLSConfig = tlsConfig
		}

		go func() {
			s.logger.Info("HTTP server starting", "port", s.cfg.HTTPPort, "mtls", s.cfg.MTLSEnabled)
			var err error
			if s.cfg.MTLSEnabled {
				err = s.httpSrv.ListenAndServeTLS(s.cfg.MTLSServerCert, s.cfg.MTLSServerKey)
			} else {
				err = s.httpSrv.ListenAndServe()
			}
			if err != nil && err != http.ErrServerClosed {
				errChan <- fmt.Errorf("http server failed: %w", err)
			}
		}()
	}

	// Start gRPC Server
	if s.cfg.EnableGRPC {
		go func() {
			s.logger.Info("gRPC server starting", "port", s.cfg.GRPCPort)
			lis, err := SystemSocket(s.cfg.GRPCPort)
			if err != nil {
				errChan <- fmt.Errorf("failed to listen grpc: %w", err)
				return
			}
			if err := s.grpcSrv.Serve(lis); err != nil {
				errChan <- fmt.Errorf("grpc server failed: %w", err)
			}
		}()
	}

	// Wait for Shutdown
	select {
	case <-ctx.Done():
		s.logger.Info("Shutting down servers...")
		return s.shutdown()
	case err := <-errChan:
		return err
	}
}

func (s *Server) shutdown() error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
	defer cancel()

	if s.httpSrv != nil {
		if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("HTTP shutdown error", "error", err)
		}
	}

	if s.grpcSrv != nil {
		s.grpcSrv.GracefulStop()
	}

	return nil
}

func loadMTLSConfig(caPath string) (*tls.Config, error) {
	caCert, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("could not read CA cert: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to append CA cert")
	}

	return &tls.Config{
		ClientCAs:  caCertPool,
		ClientAuth: tls.RequireAndVerifyClientCert,
		MinVersion: tls.VersionTLS12,
	}, nil
}

func SystemSocket(port string) (net.Listener, error) {
	return net.Listen("tcp", ":"+port)
}
