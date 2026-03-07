package server

import (
	"context"
	"fmt"
	"net"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	pb "github.com/IvanDolgov/go-musthave-metrics-tpl/internal/proto"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/storage"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// MetricsServer реализует gRPC сервис Metrics
type MetricsServer struct {
	pb.UnimplementedMetricsServer
	store         storage.Storage
	trustedSubnet string
}

// NewMetricsServer создает новый экземпляр gRPC сервера
func NewMetricsServer(store storage.Storage, trustedSubnet string) *MetricsServer {
	return &MetricsServer{
		store:         store,
		trustedSubnet: trustedSubnet,
	}
}

// UpdateMetrics обрабатывает запрос на обновление метрик
func (s *MetricsServer) UpdateMetrics(ctx context.Context, req *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
	metrics := req.GetMetrics()
	if len(metrics) == 0 {
		return nil, status.Error(codes.InvalidArgument, "empty metrics batch")
	}

	logger.Log.Debug("Received metrics batch via gRPC", zap.Int("count", len(metrics)))

	// Конвертируем protobuf метрики в модели
	modelsMetrics := make([]models.Metrics, 0, len(metrics))
	for _, m := range metrics {
		modelMetric := models.Metrics{
			ID: m.GetId(),
		}

		switch m.GetType() {
		case pb.Metric_GAUGE:
			modelMetric.MType = "gauge"
			value := m.GetValue()
			modelMetric.Value = &value
		case pb.Metric_COUNTER:
			modelMetric.MType = "counter"
			delta := m.GetDelta()
			modelMetric.Delta = &delta
		default:
			logger.Log.Warn("Unknown metric type", zap.String("type", m.GetType().String()))
			continue
		}

		modelsMetrics = append(modelsMetrics, modelMetric)
	}

	// Сохраняем метрики в хранилище
	if err := s.store.UpdateMetricsBatch(ctx, modelsMetrics); err != nil {
		logger.Log.Error("Failed to update metrics batch", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to update metrics: %v", err)
	}

	logger.Log.Debug("Successfully updated metrics batch via gRPC", zap.Int("count", len(metrics)))
	return &pb.UpdateMetricsResponse{}, nil
}

// checkIP проверяет IP адрес в метаданных
func (s *MetricsServer) checkIP(ctx context.Context) error {
	// Если подсеть не задана, пропускаем
	if s.trustedSubnet == "" {
		return nil
	}

	// Получаем метаданные из контекста
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.PermissionDenied, "missing metadata")
	}

	// Получаем IP из заголовка x-real-ip
	ips := md.Get("x-real-ip")
	if len(ips) == 0 {
		return status.Error(codes.PermissionDenied, "missing x-real-ip header")
	}

	realIP := ips[0]
	logger.Log.Debug("Checking IP", zap.String("ip", realIP), zap.String("trusted_subnet", s.trustedSubnet))

	// Используем существующую функцию из middleware для проверки IP
	if !isIPInTrustedSubnet(realIP, s.trustedSubnet) {
		return status.Errorf(codes.PermissionDenied, "IP %s not in trusted subnet %s", realIP, s.trustedSubnet)
	}

	return nil
}

// UnaryInterceptor для проверки доверенной подсети
func UnaryInterceptor(trustedSubnet string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Если подсеть не задана, пропускаем все запросы
		if trustedSubnet == "" {
			return handler(ctx, req)
		}

		// Получаем метаданные из контекста
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.PermissionDenied, "missing metadata")
		}

		// Получаем IP из заголовка x-real-ip
		ips := md.Get("x-real-ip")
		if len(ips) == 0 {
			return nil, status.Error(codes.PermissionDenied, "missing x-real-ip header")
		}

		realIP := ips[0]
		logger.Log.Debug("gRPC interceptor checking IP",
			zap.String("method", info.FullMethod),
			zap.String("ip", realIP),
			zap.String("trusted_subnet", trustedSubnet))

		// Проверяем IP
		if !isIPInTrustedSubnet(realIP, trustedSubnet) {
			return nil, status.Errorf(codes.PermissionDenied, "IP %s not in trusted subnet %s", realIP, trustedSubnet)
		}

		// IP в доверенной подсети, пропускаем запрос
		return handler(ctx, req)
	}
}

// StartGRPCServer запускает gRPC сервер
func StartGRPCServer(addr string, store storage.Storage, trustedSubnet string) (*grpc.Server, net.Listener, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to listen: %w", err)
	}

	// Создаем сервер с interceptor'ом
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(UnaryInterceptor(trustedSubnet)),
	)

	// Регистрируем сервис
	metricsServer := NewMetricsServer(store, trustedSubnet)
	pb.RegisterMetricsServer(grpcServer, metricsServer)

	logger.Log.Info("gRPC server started",
		zap.String("address", addr),
		zap.String("trusted_subnet", trustedSubnet))

	return grpcServer, lis, nil
}
