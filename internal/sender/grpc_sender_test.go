package sender

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	pb "github.com/IvanDolgov/go-musthave-metrics-tpl/internal/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// Mock gRPC server
type mockMetricsServer struct {
	pb.UnimplementedMetricsServer
	updateMetricsFunc func(ctx context.Context, req *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error)
}

func (m *mockMetricsServer) UpdateMetrics(ctx context.Context, req *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
	if m.updateMetricsFunc != nil {
		return m.updateMetricsFunc(ctx, req)
	}
	return &pb.UpdateMetricsResponse{}, nil
}

// Helper function to create test metrics
func createTestMetrics() []models.Metrics {
	gaugeValue := 123.456
	counterValue := int64(789)

	return []models.Metrics{
		{
			ID:    "test_gauge",
			MType: "gauge",
			Value: &gaugeValue,
		},
		{
			ID:    "test_counter",
			MType: "counter",
			Delta: &counterValue,
		},
	}
}

// Helper function to create test config
func createTestConfig() models.Config {
	return models.Config{
		GRPCAddress: "localhost:3200",
		RateLimit:   10,
	}
}

// Helper interface to unify testing.T and testing.B
type testHelper interface {
	Log(args ...interface{})
	Logf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
	Helper()
}

// Setup function for creating a sender with mock server
func setupTestGRPCSender(t testHelper, server pb.MetricsServer) (*GRPCMetricsSender, func()) {
	if tt, ok := t.(*testing.T); ok {
		logger.Log = zaptest.NewLogger(tt)
	} else if tb, ok := t.(*testing.B); ok {
		logger.Log = zaptest.NewLogger(tb)
	}

	// Create a listener with bufconn for in-memory testing
	listener := bufconn.Listen(1024 * 1024)

	s := grpc.NewServer()
	pb.RegisterMetricsServer(s, server)

	go func() {
		if err := s.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Logf("Server exited with error: %v", err)
		}
	}()

	// Create client connection
	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("Failed to create client connection: %v", err)
	}

	client := pb.NewMetricsClient(conn)

	sender := &GRPCMetricsSender{
		config:      createTestConfig(),
		conn:        conn,
		client:      client,
		shutdown:    make(chan struct{}),
		metricsChan: make(chan []models.Metrics, 100),
	}

	cleanup := func() {
		sender.Stop()
		sender.Wait()
		s.Stop()
		listener.Close()
	}

	return sender, cleanup
}

// TestGRPCNewMetricsSender
func TestGRPCNewMetricsSender(t *testing.T) {
	logger.Log = zaptest.NewLogger(t)

	tests := []struct {
		name        string
		config      models.Config
		expectError bool
	}{
		{
			name:        "Success with default address",
			config:      models.Config{},
			expectError: false,
		},
		{
			name: "Success with custom address",
			config: models.Config{
				GRPCAddress: "localhost:3300",
				RateLimit:   5,
			},
			expectError: false,
		},
		{
			name: "Error with invalid address",
			config: models.Config{
				GRPCAddress: "invalid:address:port",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender, err := NewGRPCMetricsSender(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, sender)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, sender)
				if sender != nil {
					defer sender.Close()
					assert.NotNil(t, sender.conn)
					assert.NotNil(t, sender.client)
					assert.NotNil(t, sender.shutdown)
					assert.NotNil(t, sender.metricsChan)
				}
			}
		})
	}
}

// TestGRPCSendMetricsBatch
func TestGRPCSendMetricsBatch(t *testing.T) {
	logger.Log = zaptest.NewLogger(t)
	sender := &GRPCMetricsSender{
		metricsChan: make(chan []models.Metrics, 1),
		shutdown:    make(chan struct{}),
	}
	defer close(sender.shutdown)

	ctx := context.Background()
	metrics := createTestMetrics()

	tests := []struct {
		name      string
		setup     func()
		expectErr bool
	}{
		{
			name: "Success send to channel",
			setup: func() {
				// Clear channel
				select {
				case <-sender.metricsChan:
				default:
				}
			},
			expectErr: false,
		},
		{
			name: "Context timeout",
			setup: func() {
				// Fill channel
				sender.metricsChan <- createTestMetrics()
			},
			expectErr: true,
		},
		{
			name: "Shutting down",
			setup: func() {
				close(sender.shutdown)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			timeoutCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
			defer cancel()

			err := sender.SendMetricsBatch(timeoutCtx, metrics)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestGRPCLifecycle
func TestGRPCLifecycle(t *testing.T) {
	mockServer := &mockMetricsServer{
		updateMetricsFunc: func(ctx context.Context, req *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
			return &pb.UpdateMetricsResponse{}, nil
		},
	}

	sender, cleanup := setupTestGRPCSender(t, mockServer)
	defer cleanup()

	// Test Start
	sender.Start()

	// Send some metrics
	metrics := createTestMetrics()
	err := sender.SendMetricsBatch(context.Background(), metrics)
	assert.NoError(t, err)

	// Test Stop
	sender.Stop()

	// Test Wait with timeout
	done := make(chan struct{})
	go func() {
		sender.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Wait timeout")
	}
}

// TestGRPCSendMetricsWithContext
func TestGRPCSendMetricsWithContext(t *testing.T) {
	tests := []struct {
		name         string
		metrics      []models.Metrics
		mockResponse func(ctx context.Context, req *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error)
		expectError  bool
	}{
		{
			name:    "Success send metrics",
			metrics: createTestMetrics(),
			mockResponse: func(ctx context.Context, req *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
				// Check metadata
				md, ok := metadata.FromOutgoingContext(ctx)
				assert.True(t, ok)
				assert.Contains(t, md, "x-real-ip")

				// Check metrics conversion
				assert.Len(t, req.Metrics, 2)
				assert.Equal(t, "test_gauge", req.Metrics[0].Id)
				assert.Equal(t, pb.Metric_GAUGE, req.Metrics[0].Type)
				assert.Equal(t, float64(123.456), req.Metrics[0].Value)

				assert.Equal(t, "test_counter", req.Metrics[1].Id)
				assert.Equal(t, pb.Metric_COUNTER, req.Metrics[1].Type)
				assert.Equal(t, int64(789), req.Metrics[1].Delta)

				return &pb.UpdateMetricsResponse{}, nil
			},
			expectError: false,
		},
		{
			name: "Success with empty metrics",
			mockResponse: func(ctx context.Context, req *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
				assert.Empty(t, req.Metrics)
				return &pb.UpdateMetricsResponse{}, nil
			},
			expectError: false,
		},
		{
			name:    "Server error",
			metrics: createTestMetrics(),
			mockResponse: func(ctx context.Context, req *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
				return nil, status.Error(codes.Internal, "internal server error")
			},
			expectError: true,
		},
		{
			name: "Invalid metric type",
			metrics: []models.Metrics{
				{
					ID:    "invalid",
					MType: "unknown",
				},
			},
			mockResponse: func(ctx context.Context, req *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
				assert.Len(t, req.Metrics, 1)
				assert.Equal(t, "invalid", req.Metrics[0].Id)
				return &pb.UpdateMetricsResponse{}, nil
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := &mockMetricsServer{
				updateMetricsFunc: tt.mockResponse,
			}

			sender, cleanup := setupTestGRPCSender(t, mockServer)
			defer cleanup()

			ctx := context.Background()
			err := sender.sendMetricsWithContext(ctx, tt.metrics)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestGRPCProcessMetrics
func TestGRPCProcessMetrics(t *testing.T) {
	received := make(chan bool)

	mockServer := &mockMetricsServer{
		updateMetricsFunc: func(ctx context.Context, req *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
			assert.Len(t, req.Metrics, 2)
			received <- true
			return &pb.UpdateMetricsResponse{}, nil
		},
	}

	sender, cleanup := setupTestGRPCSender(t, mockServer)
	defer cleanup()

	// Start processor
	sender.wg.Add(1)
	go sender.processMetrics()

	// Send metrics
	metrics := createTestMetrics()
	sender.metricsChan <- metrics

	// Wait for processing
	select {
	case <-received:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Metrics not processed")
	}

	// Test shutdown
	sender.shutdown <- struct{}{}
	time.Sleep(100 * time.Millisecond)
}

// TestGRPCDrainAndSend
func TestGRPCDrainAndSend(t *testing.T) {
	var receivedCount int
	mockServer := &mockMetricsServer{
		updateMetricsFunc: func(ctx context.Context, req *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
			receivedCount += len(req.Metrics)
			return &pb.UpdateMetricsResponse{}, nil
		},
	}

	sender, cleanup := setupTestGRPCSender(t, mockServer)
	defer cleanup()

	// Fill channel with metrics
	metrics := createTestMetrics()
	for i := 0; i < 3; i++ {
		sender.metricsChan <- metrics
	}

	// Call drainAndSend
	sender.drainAndSend()

	// Check that all metrics were sent
	assert.Equal(t, 6, receivedCount) // 3 batches * 2 metrics each
	assert.Empty(t, sender.metricsChan)
}

// TestGRPCInitLocalIP
func TestGRPCInitLocalIP(t *testing.T) {
	sender := &GRPCMetricsSender{}

	// First call
	sender.initLocalIP()
	firstIP := sender.localIP

	// Second call should not change
	sender.initLocalIP()
	assert.Equal(t, firstIP, sender.localIP)

	// Check that IP is set (could be empty if network unavailable)
	t.Logf("Local IP: %s", sender.localIP)
}

// TestGRPCClose
func TestGRPCClose(t *testing.T) {
	tests := []struct {
		name      string
		hasConn   bool
		expectErr bool
	}{
		{
			name:      "Close with connection",
			hasConn:   true,
			expectErr: false,
		},
		{
			name:      "Close without connection",
			hasConn:   false,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &GRPCMetricsSender{}
			if tt.hasConn {
				// Create a dummy connection
				conn, err := grpc.NewClient("localhost:3200",
					grpc.WithTransportCredentials(insecure.NewCredentials()))
				require.NoError(t, err)
				sender.conn = conn
			}

			err := sender.Close()
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// BenchmarkGRPCSendMetricsWithContext
func BenchmarkGRPCSendMetricsWithContext(b *testing.B) {
	mockServer := &mockMetricsServer{
		updateMetricsFunc: func(ctx context.Context, req *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
			return &pb.UpdateMetricsResponse{}, nil
		},
	}

	sender, cleanup := setupTestGRPCSender(b, mockServer)
	defer cleanup()

	metrics := createTestMetrics()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := sender.sendMetricsWithContext(ctx, metrics)
		if err != nil {
			b.Fatal(err)
		}
	}
}
