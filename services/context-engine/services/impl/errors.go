package impl

import (
	"context"
	"errors"
	"fmt"

	"github.com/hibiken/asynq"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// isPermanentError returns true for errors that will never succeed on retry:
// resource deleted / invalid / unauthorized. Distinguished from transient
// failures like Unavailable, DeadlineExceeded, or ResourceExhausted.
func isPermanentError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	switch st.Code() {
	case codes.NotFound,
		codes.PermissionDenied,
		codes.Unauthenticated,
		codes.InvalidArgument,
		codes.FailedPrecondition,
		codes.AlreadyExists,
		codes.OutOfRange:
		return true
	}
	return false
}

// skipRetryIfPermanent wraps a permanent error so it chains asynq.SkipRetry
// (archiving the task) while preserving the original message for logging.
// Transient errors pass through unchanged so asynq retries them.
func skipRetryIfPermanent(err error) error {
	if err == nil {
		return nil
	}
	if isPermanentError(err) {
		return fmt.Errorf("%s: %w", err.Error(), asynq.SkipRetry)
	}
	return err
}
