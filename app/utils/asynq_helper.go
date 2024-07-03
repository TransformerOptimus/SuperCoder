package utils

import (
	"github.com/hibiken/asynq"
	"math"
	"time"
)

func ExponentialBackoff(n int, e error, task *asynq.Task) time.Duration {
	return time.Duration(math.Pow(2, float64(n))) * time.Second
}
