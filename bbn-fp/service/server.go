package service

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	fpcfg "github.com/Manta-Network/manta-fp/bbn-fp/config"
	"github.com/Manta-Network/manta-fp/metrics"

	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/signal"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// Server is the main daemon construct for the Finality Provider server. It handles
// spinning up the RPC sever, the database, and any other components that the
// Taproot Asset server needs to function.
type Server struct {
	started int32

	cfg    *fpcfg.Config
	logger *zap.Logger

	rpcServer   *rpcServer
	db          kvdb.Backend
	interceptor signal.Interceptor

	metricsServer *metrics.Server
	lis           net.Listener
	grpcServer    *grpc.Server

	quit chan struct{}
}

// NewFinalityproviderServer creates a new server with the given config.
func NewFinalityProviderServer(cfg *fpcfg.Config, l *zap.Logger, fpa *FinalityProviderApp, db kvdb.Backend, sig signal.Interceptor) *Server {
	return &Server{
		cfg:         cfg,
		logger:      l,
		rpcServer:   newRPCServer(fpa),
		db:          db,
		interceptor: sig,
		quit:        make(chan struct{}, 1),
	}
}

func (s *Server) StartFinalityProviderServer() error {
	if atomic.AddInt32(&s.started, 1) != 1 {
		return nil
	}

	// Start the metrics server.
	promAddr, err := s.cfg.Metrics.Address()
	if err != nil {
		return fmt.Errorf("failed to get prometheus address: %w", err)
	}
	s.metricsServer = metrics.Start(promAddr, s.logger)

	listenAddr := s.cfg.RPCListener
	// we create listeners from the RPCListeners defined
	// in the config.
	s.lis, err = net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", listenAddr, err)
	}

	grpcServer := grpc.NewServer()
	s.grpcServer = grpcServer

	if err := s.rpcServer.RegisterWithGrpcServer(grpcServer); err != nil {
		return fmt.Errorf("failed to register gRPC server: %w", err)
	}

	// All the necessary parts have been registered, so we can
	// actually start listening for requests.
	s.startGrpcListen(grpcServer, []net.Listener{s.lis})

	s.logger.Info("Finality Provider Daemon is fully active!")

	return nil
}

// RunUntilShutdown runs the main EOTS manager server loop until a signal is
// received to shut down the process.
func (s *Server) RunUntilShutdown() error {

	defer func() {
		s.logger.Info("Shutdown complete")
	}()

	defer func() {
		s.logger.Info("Closing database...")
		if err := s.db.Close(); err != nil {
			s.logger.Error(fmt.Sprintf("Failed to close database: %v", err)) // Log the error
		} else {
			s.logger.Info("Database closed")
		}
		s.metricsServer.Stop(context.Background())
		s.logger.Info("Metrics server stopped")
	}()

	defer func() {
		if err := s.lis.Close(); err != nil {
			s.logger.Error(fmt.Sprintf("Failed to close network listener: %v", err))
		}
	}()

	defer s.grpcServer.Stop()

	// Wait for shutdown signal from either a graceful server stop or from
	// the interrupt handler.
	<-s.interceptor.ShutdownChannel()

	return nil
}

// startGrpcListen starts the GRPC server on the passed listeners.
func (s *Server) startGrpcListen(grpcServer *grpc.Server, listeners []net.Listener) {
	// Use a WaitGroup, so we can be sure the instructions on how to input the
	// password is the last thing to be printed to the console.
	var wg sync.WaitGroup

	for _, lis := range listeners {
		wg.Add(1)
		go func(lis net.Listener) {
			s.logger.Info("RPC server listening", zap.String("address", lis.Addr().String()))

			// Close the ready chan to indicate we are listening.
			defer lis.Close()

			wg.Done()
			_ = grpcServer.Serve(lis)
		}(lis)
	}

	// Wait for gRPC servers to be up running.
	wg.Wait()
}
