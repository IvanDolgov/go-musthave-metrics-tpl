-- Создание таблицы для gauge метрик
CREATE TABLE IF NOT EXISTS gauge_metrics (
    name VARCHAR(255) PRIMARY KEY,
    value DOUBLE PRECISION NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

-- Создание таблицы для counter метрик
CREATE TABLE IF NOT EXISTS counter_metrics (
    name VARCHAR(255) PRIMARY KEY,
    value BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

-- Индексы для оптимизации запросов
CREATE INDEX IF NOT EXISTS idx_gauge_metrics_name ON gauge_metrics(name);
CREATE INDEX IF NOT EXISTS idx_counter_metrics_name ON counter_metrics(name);
CREATE INDEX IF NOT EXISTS idx_gauge_metrics_updated ON gauge_metrics(updated_at);
CREATE INDEX IF NOT EXISTS idx_counter_metrics_updated ON counter_metrics(updated_at);