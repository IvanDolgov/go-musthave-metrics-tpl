-- Откат миграции - удаление таблиц
DROP TABLE IF EXISTS counter_metrics;
DROP TABLE IF EXISTS gauge_metrics;
DROP TABLE IF EXISTS schema_migrations;