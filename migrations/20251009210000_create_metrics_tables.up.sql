-- Создание таблиц для метрик
CREATE TABLE IF NOT EXISTS gauge_metrics (
    name VARCHAR(255) PRIMARY KEY,
    value DOUBLE PRECISION NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE TABLE IF NOT EXISTS counter_metrics (
    name VARCHAR(255) PRIMARY KEY,
    value BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

-- Индексы для оптимизации
CREATE INDEX IF NOT EXISTS idx_gauge_metrics_name ON gauge_metrics(name);
CREATE INDEX IF NOT EXISTS idx_counter_metrics_name ON counter_metrics(name);
CREATE INDEX IF NOT EXISTS idx_gauge_metrics_updated ON gauge_metrics(updated_at);
CREATE INDEX IF NOT EXISTS idx_counter_metrics_updated ON counter_metrics(updated_at);

-- Таблица для отслеживания миграций
CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT PRIMARY KEY,
    applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Записываем версию миграции
INSERT INTO schema_migrations (version) VALUES (1) ON CONFLICT (version) DO NOTHING;