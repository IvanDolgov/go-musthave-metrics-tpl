package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/agent"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/buildinfo"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/config"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/sender"
	"go.uber.org/zap"
)

// Глобальные переменные для версии сборки
var (
	buildVersion string
	buildDate    string
	buildCommit  string
)

func run(ctx context.Context, cfg models.Config) error {
	// Вывод информации о сборке
	buildinfo.Print()

	var sdr agent.MetricsSender
	var err error

	// Выбираем тип отправителя в зависимости от конфигурации
	if cfg.UseGRPC {
		logger.Log.Info("Using gRPC sender", zap.String("address", cfg.GRPCAddress))
		sdr, err = sender.NewGRPCMetricsSender(cfg)
	} else {
		logger.Log.Info("Using HTTP sender", zap.String("address", cfg.Address))
		sdr, err = sender.NewHTTPMetricsSender(cfg)
	}

	if err != nil {
		return fmt.Errorf("failed to create metrics sender: %w", err)
	}

	// Запускаем обработчик отправки
	sdr.Start()
	defer sdr.Stop()

	// Создаем агента
	metricsAgent := agent.NewMetricsAgent(cfg, sdr)

	// Запускаем агента
	metricsAgent.Start()
	defer metricsAgent.Stop()

	// Ожидаем завершения по сигналу
	<-ctx.Done()
	logger.Log.Info("Context cancelled, initiating graceful shutdown...")

	// Даем время на завершение текущих операций (максимум 30 секунд)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Ожидаем завершения всех горутин
	done := make(chan struct{})
	go func() {
		metricsAgent.Wait()
		sdr.Wait() // Здесь используем sdr.Wait(), а не sender.Wait
		close(done)
	}()

	select {
	case <-done:
		logger.Log.Info("All goroutines finished successfully")
	case <-shutdownCtx.Done():
		logger.Log.Warn("Shutdown timeout exceeded, some goroutines may not have finished")
	}

	return nil
}

func main() {
	// Получаем конфигурацию
	cfg := config.ParseAgentFlags()

	// Инициализируем логер
	if err := logger.Initialize("info"); err != nil {
		fmt.Fprintf(os.Stderr, "Logger initialization error: %v\n", err)
		os.Exit(1)
	}
	defer logger.Log.Sync()

	// Создаем контекст с возможностью отмены
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Канал для сигналов ОС
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	// Запускаем горутину для обработки сигналов
	go func() {
		sig := <-sigChan
		logger.Log.Info("Received shutdown signal", zap.String("signal", sig.String()))
		cancel()
	}()

	// Запускаем приложение с контекстом
	if err := run(ctx, cfg); err != nil {
		logger.Log.Error("Application error", zap.Error(err))
		os.Exit(1)
	}

	logger.Log.Info("Agent shutdown complete")
}
