package controllers

import (
	"context"
	"net/http"

	"moa-chain/explorer/service"
)

// Server is the explorer HTTP server.
type Server struct {
	svc *service.ExplorerService
	srv *http.Server
}

// NewServer creates an HTTP server that serves the explorer API on addr.
func NewServer(svc *service.ExplorerService, addr string) *Server {
	mux := http.NewServeMux()
	s := &Server{
		svc: svc,
		srv: &http.Server{Addr: addr, Handler: mux},
	}
	s.registerRoutes(mux)
	return s
}

// Handler returns the underlying http.Handler. Used in tests to invoke handlers
// directly without starting a real TCP listener.
func (s *Server) Handler() http.Handler { return s.srv.Handler }

// Start begins serving HTTP requests. Blocks until stopped or fatal error.
// Returns http.ErrServerClosed on clean shutdown.
func (s *Server) Start() error { return s.srv.ListenAndServe() }

// Stop gracefully shuts down the server without interrupting active connections.
func (s *Server) Stop(ctx context.Context) error { return s.srv.Shutdown(ctx) }

func (s *Server) routes() []RequestHandler {
	return []RequestHandler{
		newRequestHandler("GET", "/api/v1/health", s.handleHealth),
		newRequestHandler("GET", "/api/v1/blocks/{hash}", s.handleBlock),
		newRequestHandler("GET", "/api/v1/rounds/{round}", s.handleRound),
		newRequestHandler("GET", "/api/v1/transactions", s.handleTransactions),
		newRequestHandler("GET", "/api/v1/transactions/{hash}", s.handleTransaction),
	}
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	for _, rh := range s.routes() {
		mux.HandleFunc(rh.GetHttpMethod()+" "+rh.GetPath(), rh.GetHandler())
	}
}
