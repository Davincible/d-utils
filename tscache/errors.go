package tscache

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidConfig indicates an invalid configuration
	ErrInvalidConfig = errors.New("invalid configuration")

	// ErrStorageCorrupted indicates corrupted storage
	ErrStorageCorrupted = errors.New("storage corrupted")

	// ErrPartitionNotFound indicates a partition was not found
	ErrPartitionNotFound = errors.New("partition not found")

	// ErrItemNotFound indicates an item was not found
	ErrItemNotFound = errors.New("item not found")

	// ErrInvalidTimeRange indicates an invalid time range
	ErrInvalidTimeRange = errors.New("invalid time range")
)

// StorageError wraps storage-related errors
type StorageError struct {
	Op   string // Operation being performed
	Path string // File path if applicable
	Err  error  // Underlying error
}

func (e *StorageError) Error() string {
	if e.Path == "" {
		return fmt.Sprintf("storage error during %s: %v", e.Op, e.Err)
	}
	return fmt.Sprintf("storage error during %s on %s: %v", e.Op, e.Path, e.Err)
}

func (e *StorageError) Unwrap() error {
	return e.Err
}

// PartitionError wraps partition-related errors
type PartitionError struct {
	StartTime int64 // Partition start time
	Op        string
	Err       error
}

func (e *PartitionError) Error() string {
	return fmt.Sprintf("partition error (start: %d) during %s: %v", e.StartTime, e.Op, e.Err)
}

func (e *PartitionError) Unwrap() error {
	return e.Err
}

// WrapStorageError wraps an error with storage context
func WrapStorageError(op string, path string, err error) error {
	return &StorageError{
		Op:   op,
		Path: path,
		Err:  err,
	}
}

// WrapPartitionError wraps an error with partition context
func WrapPartitionError(startTime int64, op string, err error) error {
	return &PartitionError{
		StartTime: startTime,
		Op:        op,
		Err:       err,
	}
}
