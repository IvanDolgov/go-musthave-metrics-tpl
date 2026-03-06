package sender

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	pb "github.com/IvanDolgov/go-musthave-metrics-tpl/internal/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// GRPCMetricsSender реализация отправки метрик по gRPC
type GRPCMetricsSender struct {
	config      models.Config
	conn        *grpc.ClientConn
	client      pb.MetricsClient
	wg          sync.WaitGroup
	shutdown    chan struct{}
	metricsChan chan []models.Metrics
	localIP     string
	ipOnce      sync.Once
}

// NewGRPCMetricsSender создает новый экземпляр GRPCMetricsSender
func NewGRPCMetricsSender(cfg models.Config) (*GRPCMetricsSender, error) {
	// Определяем адрес gRPC сервера
	grpcAddr := cfg.GRPCAddress
	if grpcAddr == "" {
		grpcAddr = "localhost:3200"
	}

	// Устанавливаем соединение с gRPC сервером
	conn, err := grpc.NewClient(
		grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
	}

	client := pb.NewMetricsClient(conn)

	sender := &GRPCMetricsSender{
		config:      cfg,
		conn:        conn,
		client:      client,
		shutdown:    make(chan struct{}),
		metricsChan: make(chan []models.Metrics, 100),
	}

	logger.Log.Info("gRPC metrics sender created",
		zap.String("address", grpcAddr),
		zap.Int64("rate_limit", cfg.RateLimit))

	return sender, nil
}

// initLocalIP инициализирует локальный IP адрес
func (s *GRPCMetricsSender) initLocalIP() {
	s.ipOnce.Do(func() {
		ip, err := getLocalIP()
		if err != nil {
			logger.Log.Warn("Failed to get local IP", zap.Error(err))
			s.localIP = ""
		} else {
			s.localIP = ip
			logger.Log.Debug("Local IP detected", zap.String("ip", s.localIP))
		}
	})
}

// Start запускает обработчик отправки метрик
func (s *GRPCMetricsSender) Start() {
	s.wg.Add(1)
	go s.processMetrics()
	logger.Log.Info("gRPC metrics sender started")
}

// Stop останавливает обработчик и ожидает завершения всех отправок
func (s *GRPCMetricsSender) Stop() {
	logger.Log.Info("Stopping gRPC metrics sender, waiting for pending requests...")
	close(s.shutdown)
	s.wg.Wait()
	if s.conn != nil {
		s.conn.Close()
	}
	logger.Log.Info("gRPC metrics sender stopped")
}

// Wait ожидает завершения всех горутин отправителя
func (s *GRPCMetricsSender) Wait() {
	s.wg.Wait()
}

// SendMetricsBatch отправляет батч метрик на сервер (неблокирующий вызов)
func (s *GRPCMetricsSender) SendMetricsBatch(ctx context.Context, metrics []models.Metrics) error {
	select {
	case s.metricsChan <- metrics:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.shutdown:
		return fmt.Errorf("sender is shutting down")
	}
}

// processMetrics обрабатывает метрики из канала и отправляет их на сервер
func (s *GRPCMetricsSender) processMetrics() {
	defer s.wg.Done()

	for {
		select {
		case metrics, ok := <-s.metricsChan:
			if !ok {
				logger.Log.Info("Metrics channel closed, stopping processor")
				return
			}
			err := s.sendMetrics(metrics)
			if err != nil {
				logger.Log.Error("Failed to send metrics batch via gRPC",
					zap.Error(err),
					zap.Int("metrics_count", len(metrics)))
			}
		case <-s.shutdown:
			logger.Log.Info("Processing remaining metrics before shutdown")
			s.drainAndSend()
			return
		}
	}
}

// drainAndSend отправляет все оставшиеся метрики при завершении
func (s *GRPCMetricsSender) drainAndSend() {
	remaining := len(s.metricsChan)
	if remaining > 0 {
		logger.Log.Info("Sending remaining metrics via gRPC", zap.Int("count", remaining))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		for i := 0; i < remaining; i++ {
			select {
			case metrics := <-s.metricsChan:
				err := s.sendMetricsWithContext(ctx, metrics)
				if err != nil {
					logger.Log.Error("Failed to send metrics during shutdown via gRPC",
						zap.Error(err),
						zap.Int("metrics_count", len(metrics)))
				}
			case <-ctx.Done():
				logger.Log.Warn("Shutdown timeout exceeded while sending remaining metrics")
				return
			default:
				return
			}
		}
	}
}

// sendMetrics отправляет метрики на сервер через gRPC
func (s *GRPCMetricsSender) sendMetrics(metrics []models.Metrics) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.sendMetricsWithContext(ctx, metrics)
}

// sendMetricsWithContext отправляет метрики на сервер через gRPC с заданным контекстом
func (s *GRPCMetricsSender) sendMetricsWithContext(ctx context.Context, metrics []models.Metrics) error {
	startTime := time.Now()

	// Конвертируем модели метрик в protobuf
	pbMetrics := make([]*pb.Metric, 0, len(metrics))
	for _, m := range metrics {
		pbMetric := &pb.Metric{
			Id: m.ID,
		}

		// Устанавливаем тип метрики
		if m.MType == "gauge" {
			pbMetric.Type = pb.Metric_GAUGE
			if m.Value != nil {
				pbMetric.Value = *m.Value
			}
		} else if m.MType == "counter" {
			pbMetric.Type = pb.Metric_COUNTER
			if m.Delta != nil {
				pbMetric.Delta = *m.Delta
			}
		}

		pbMetrics = append(pbMetrics, pbMetric)
	}

	// Создаем запрос (используем обычную структуру, не _builder)
	request := &pb.UpdateMetricsRequest{
		Metrics: pbMetrics,
	}

	// Добавляем IP в метаданные
	s.initLocalIP()
	md := metadata.New(map[string]string{
		"x-real-ip": s.localIP,
	})
	ctx = metadata.NewOutgoingContext(ctx, md)

	// Отправляем запрос
	_, err := s.client.UpdateMetrics(ctx, request)
	if err != nil {
		return fmt.Errorf("gRPC call failed: %w", err)
	}

	duration := time.Since(startTime)
	logger.Log.Debug("Successfully sent metrics batch via gRPC",
		zap.Int("metrics_count", len(metrics)),
		zap.Duration("duration", duration),
		zap.String("local_ip", s.localIP),
	)

	return nil
}

// Close закрывает соединение с gRPC сервером
func (s *GRPCMetricsSender) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}
