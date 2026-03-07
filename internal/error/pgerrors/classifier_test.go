package pgerrors

import (
	"errors"
	"testing"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/retry"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func TestPostgresErrorClassifier_Classify(t *testing.T) {
	classifier := NewPostgresErrorClassifier()

	tests := []struct {
		name     string
		err      error
		expected retry.ErrorClassification
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: retry.NonRetriable,
		},
		{
			name:     "non-postgres error",
			err:      errors.New("some random error"),
			expected: retry.NonRetriable,
		},
		{
			name: "connection exception - general",
			err: &pgconn.PgError{
				Code: pgerrcode.ConnectionException,
			},
			expected: retry.Retriable,
		},
		{
			name: "connection exception - does not exist",
			err: &pgconn.PgError{
				Code: pgerrcode.ConnectionDoesNotExist,
			},
			expected: retry.Retriable,
		},
		{
			name: "connection exception - failure",
			err: &pgconn.PgError{
				Code: pgerrcode.ConnectionFailure,
			},
			expected: retry.Retriable,
		},
		{
			name: "transaction rollback - general",
			err: &pgconn.PgError{
				Code: pgerrcode.TransactionRollback,
			},
			expected: retry.Retriable,
		},
		{
			name: "transaction rollback - serialization failure",
			err: &pgconn.PgError{
				Code: pgerrcode.SerializationFailure,
			},
			expected: retry.Retriable,
		},
		{
			name: "transaction rollback - deadlock detected",
			err: &pgconn.PgError{
				Code: pgerrcode.DeadlockDetected,
			},
			expected: retry.Retriable,
		},
		{
			name: "insufficient resources - general",
			err: &pgconn.PgError{
				Code: pgerrcode.InsufficientResources,
			},
			expected: retry.Retriable,
		},
		{
			name: "insufficient resources - disk full",
			err: &pgconn.PgError{
				Code: pgerrcode.DiskFull,
			},
			expected: retry.Retriable,
		},
		{
			name: "insufficient resources - out of memory",
			err: &pgconn.PgError{
				Code: pgerrcode.OutOfMemory,
			},
			expected: retry.Retriable,
		},
		{
			name: "insufficient resources - too many connections",
			err: &pgconn.PgError{
				Code: pgerrcode.TooManyConnections,
			},
			expected: retry.Retriable,
		},
		{
			name: "operator intervention - general",
			err: &pgconn.PgError{
				Code: pgerrcode.OperatorIntervention,
			},
			expected: retry.Retriable,
		},
		{
			name: "operator intervention - query canceled",
			err: &pgconn.PgError{
				Code: pgerrcode.QueryCanceled,
			},
			expected: retry.Retriable,
		},
		{
			name: "cannot connect now",
			err: &pgconn.PgError{
				Code: "57P03",
			},
			expected: retry.Retriable,
		},
		{
			name: "system error - general",
			err: &pgconn.PgError{
				Code: pgerrcode.SystemError,
			},
			expected: retry.Retriable,
		},
		{
			name: "system error - IO error",
			err: &pgconn.PgError{
				Code: pgerrcode.IOError,
			},
			expected: retry.Retriable,
		},
		{
			name: "syntax error - non-retriable",
			err: &pgconn.PgError{
				Code: pgerrcode.SyntaxError,
			},
			expected: retry.NonRetriable,
		},
		{
			name: "unique violation - non-retriable",
			err: &pgconn.PgError{
				Code: pgerrcode.UniqueViolation,
			},
			expected: retry.NonRetriable,
		},
		{
			name: "foreign key violation - non-retriable",
			err: &pgconn.PgError{
				Code: pgerrcode.ForeignKeyViolation,
			},
			expected: retry.NonRetriable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.Classify(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPostgresErrorClassifier_Classify_WrappedErrors(t *testing.T) {
	classifier := NewPostgresErrorClassifier()

	// Создаем pg ошибку
	pgErr := &pgconn.PgError{
		Code: pgerrcode.ConnectionFailure,
	}

	// Заворачиваем её в несколько слоев
	wrappedErr := errors.New("some context: " + pgErr.Error())
	wrappedErr = errors.Join(wrappedErr, pgErr)

	// Должна классифицироваться как retriable, т.к. errors.As найдет pgErr
	result := classifier.Classify(wrappedErr)
	assert.Equal(t, retry.Retriable, result, "should extract pg error from wrapped error chain")
}

func TestPostgresErrorClassifier_IsConnectionError(t *testing.T) {
	classifier := NewPostgresErrorClassifier()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "non-postgres error",
			err:      errors.New("random error"),
			expected: false,
		},
		{
			name: "connection exception - general",
			err: &pgconn.PgError{
				Code: pgerrcode.ConnectionException,
			},
			expected: true,
		},
		{
			name: "connection exception - does not exist",
			err: &pgconn.PgError{
				Code: pgerrcode.ConnectionDoesNotExist,
			},
			expected: true,
		},
		{
			name: "connection exception - failure",
			err: &pgconn.PgError{
				Code: pgerrcode.ConnectionFailure,
			},
			expected: true,
		},
		{
			name: "connection exception - with class code",
			err: &pgconn.PgError{
				Code: "08000", // Connection exception class
			},
			expected: true,
		},
		{
			name: "non-connection error - syntax error",
			err: &pgconn.PgError{
				Code: pgerrcode.SyntaxError,
			},
			expected: false,
		},
		{
			name: "non-connection error - transaction rollback",
			err: &pgconn.PgError{
				Code: pgerrcode.TransactionRollback,
			},
			expected: false,
		},
		{
			name: "non-connection error - unique violation",
			err: &pgconn.PgError{
				Code: pgerrcode.UniqueViolation,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.IsConnectionError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPostgresErrorClassifier_IsTransactionError(t *testing.T) {
	classifier := NewPostgresErrorClassifier()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "non-postgres error",
			err:      errors.New("random error"),
			expected: false,
		},
		{
			name: "transaction rollback - general",
			err: &pgconn.PgError{
				Code: pgerrcode.TransactionRollback,
			},
			expected: true,
		},
		{
			name: "transaction rollback - serialization failure",
			err: &pgconn.PgError{
				Code: pgerrcode.SerializationFailure,
			},
			expected: true,
		},
		{
			name: "transaction rollback - deadlock detected",
			err: &pgconn.PgError{
				Code: pgerrcode.DeadlockDetected,
			},
			expected: true,
		},
		{
			name: "transaction error - with class code",
			err: &pgconn.PgError{
				Code: "40002", // Transaction rollback subclass
			},
			expected: true,
		},
		{
			name: "non-transaction error - connection exception",
			err: &pgconn.PgError{
				Code: pgerrcode.ConnectionException,
			},
			expected: false,
		},
		{
			name: "non-transaction error - syntax error",
			err: &pgconn.PgError{
				Code: pgerrcode.SyntaxError,
			},
			expected: false,
		},
		{
			name: "non-transaction error - unique violation",
			err: &pgconn.PgError{
				Code: pgerrcode.UniqueViolation,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.IsTransactionError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClassifyPgError_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		pgErr    *pgconn.PgError
		expected retry.ErrorClassification
	}{
		{
			name: "empty code",
			pgErr: &pgconn.PgError{
				Code: "",
			},
			expected: retry.NonRetriable,
		},
		{
			name: "invalid code format",
			pgErr: &pgconn.PgError{
				Code: "X",
			},
			expected: retry.NonRetriable,
		},
		{
			name: "unknown connection subclass but known class",
			pgErr: &pgconn.PgError{
				Code: "08999", // Connection exception class with unknown subclass
			},
			expected: retry.Retriable,
		},
		{
			name: "unknown transaction subclass but known class",
			pgErr: &pgconn.PgError{
				Code: "40999", // Transaction rollback class with unknown subclass
			},
			expected: retry.Retriable,
		},
		{
			name: "unknown class code",
			pgErr: &pgconn.PgError{
				Code: "99999",
			},
			expected: retry.NonRetriable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyPgError(tt.pgErr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func BenchmarkPostgresErrorClassifier_Classify(b *testing.B) {
	classifier := NewPostgresErrorClassifier()
	err := &pgconn.PgError{
		Code: pgerrcode.ConnectionFailure,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		classifier.Classify(err)
	}
}
