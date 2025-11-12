package pgerrors

import (
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

// PGErrorClassification тип для классификации ошибок
type PGErrorClassification int

const (
	// NonRetriable - операцию не следует повторять
	NonRetriable PGErrorClassification = iota

	// Retriable - операцию можно повторить
	Retriable
)

// PostgresErrorClassifier классификатор ошибок PostgreSQL
type PostgresErrorClassifier struct{}

func NewPostgresErrorClassifier() *PostgresErrorClassifier {
	return &PostgresErrorClassifier{}
}

// Classify классифицирует ошибку и возвращает PGErrorClassification
func (c *PostgresErrorClassifier) Classify(err error) PGErrorClassification {
	if err == nil {
		return NonRetriable
	}

	// Проверяем и конвертируем в pgconn.PgError, если это возможно
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return classifyPgError(pgErr)
	}

	// По умолчанию считаем ошибку неповторяемой
	return NonRetriable
}

func classifyPgError(pgErr *pgconn.PgError) PGErrorClassification {
	// Коды ошибок PostgreSQL: https://www.postgresql.org/docs/current/errcodes-appendix.html

	switch pgErr.Code {
	// Класс 08 - Ошибки соединения (Connection Exception)
	case pgerrcode.ConnectionException,
		pgerrcode.ConnectionDoesNotExist,
		pgerrcode.ConnectionFailure:
		return Retriable

	// Класс 40 - Откат транзакции (Transaction Rollback)
	case pgerrcode.TransactionRollback, // 40000
		pgerrcode.SerializationFailure, // 40001
		pgerrcode.DeadlockDetected:     // 40P01
		return Retriable

	// Класс 53 - Недостаточно ресурсов (Insufficient Resources)
	case pgerrcode.InsufficientResources,
		pgerrcode.DiskFull,
		pgerrcode.OutOfMemory,
		pgerrcode.TooManyConnections:
		return Retriable

	// Класс 57 - Администратор запретил операцию (Operator Intervention)
	case pgerrcode.OperatorIntervention,
		pgerrcode.QueryCanceled:
		return Retriable

	// Класс 57P03 - Cannot connect now (база в режиме standby и т.д.)
	case "57P03": // Cannot connect now
		return Retriable

	// Класс 58 - System errors (system errors)
	case pgerrcode.SystemError,
		pgerrcode.IOError:
		return Retriable

	default:
		// Проверяем по классу ошибки (первые 2 символа кода)
		errorClass := pgErr.Code[0:2]

		switch errorClass {
		case "08": // Connection Exception
			return Retriable
		case "40": // Transaction Rollback
			return Retriable
		case "53": // Insufficient Resources
			return Retriable
		case "57": // Operator Intervention
			return Retriable
		case "58": // System Error
			return Retriable
		default:
			return NonRetriable
		}
	}
}

// IsConnectionError проверяет, является ли ошибка ошибкой соединения
func (c *PostgresErrorClassifier) IsConnectionError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// Класс 08 - ошибки соединения
		return pgErr.Code[0:2] == "08"
	}
	return false
}

// IsTransactionError проверяет, является ли ошибка ошибкой транзакции
func (c *PostgresErrorClassifier) IsTransactionError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// Класс 40 - ошибки транзакции
		return pgErr.Code[0:2] == "40"
	}
	return false
}
