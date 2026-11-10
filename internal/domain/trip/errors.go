package trip

import (
	"errors"
	"fmt"
)

// Доменные ошибки. Про HTTP-коды домен не знает: перевод в ответ делает
// транспорт, в одном месте (internal/http/domainerror)
var (
	ErrTripNotFound  = errors.New("trip not found")
	ErrTripCompleted = errors.New("trip already completed")
	ErrDriverBusy    = errors.New("driver already has an active trip")
)

// ValidationError: данные поездки нарушают правило. Несёт поле и причину,
// чтобы клиент понял, что исправить
type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}

func newValidationError(field, reason string) *ValidationError {
	return &ValidationError{Field: field, Reason: reason}
}
