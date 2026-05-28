package handlers

import "errors"

var (
	ErrOrderNotFound          = errors.New("order not found")
	ErrOrderCannotBeCancelled = errors.New("order cannot be cancelled")
)
