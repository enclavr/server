package grpc

import (
	"context"
	"testing"

	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestHealthServer_Check(t *testing.T) {
	server := &HealthServer{}
	resp, err := server.Check(context.Background(), &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("expected SERVING status, got %v", resp.Status)
	}
}

func TestHealthServer_Watch(t *testing.T) {
	server := &HealthServer{}
	err := server.Watch(&healthpb.HealthCheckRequest{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMetricsServer_Check(t *testing.T) {
	server := &MetricsServer{}
	resp, err := server.Check(context.Background(), &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("expected SERVING status, got %v", resp.Status)
	}
}

func TestStartGRPCServer_InvalidPort(t *testing.T) {
	err := StartGRPCServer("invalid-port", "", "")
	if err == nil {
		t.Error("expected error with invalid port")
	}
}

func TestStartGRPCServer_InvalidCertFile(t *testing.T) {
	err := StartGRPCServer("59999", "/nonexistent/cert.pem", "/nonexistent/key.pem")
	if err == nil {
		t.Error("expected error with invalid cert file")
	}
}
