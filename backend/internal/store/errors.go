package store

import "errors"

var (
	ErrJobNotFound  = errors.New("job not found")
	ErrTaskNotFound = errors.New("task not found")
)
