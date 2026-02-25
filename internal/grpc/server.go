package grpc

import (
	"context"
	"net"

	"github.com/enclavr/server/internal/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type HealthServer struct {
	healthpb.UnimplementedHealthServer
}

func (s *HealthServer) Check(ctx context.Context, req *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func (s *HealthServer) Watch(req *healthpb.HealthCheckRequest, srv healthpb.Health_WatchServer) error {
	return nil
}

type MetricsServer struct {
	healthpb.UnimplementedHealthServer
}

func (s *MetricsServer) Check(ctx context.Context, req *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	metrics.HTTPRequestsTotal.WithLabelValues("grpc", "health", "200").Inc()
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func StartGRPCServer(port string, certFile string, keyFile string) error {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}

	var opts []grpc.ServerOption
	if certFile != "" && keyFile != "" {
		creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
		if err != nil {
			return err
		}
		opts = append(opts, grpc.Creds(creds))
	}

	grpcServer := grpc.NewServer(opts...)
	healthpb.RegisterHealthServer(grpcServer, &HealthServer{})
	healthpb.RegisterHealthServer(grpcServer, &MetricsServer{})

	return grpcServer.Serve(lis)
}
