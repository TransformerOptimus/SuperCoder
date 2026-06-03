package asynq_client

import (
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/hibiken/asynqmon"
	"go.uber.org/zap"
)

func NewAsynqClient(redisClientOpt asynq.RedisClientOpt) *asynq.Client {
	return asynq.NewClient(redisClientOpt)
}

func NewAsynqInspector(redisClientOpt asynq.RedisClientOpt) *asynq.Inspector {
	return asynq.NewInspector(redisClientOpt)
}

func NewAsynqServer(redisClientOpt asynq.RedisClientOpt, logger *zap.Logger) *asynq.Server {
	return NewAsynqServerWithShutdownTimeout(redisClientOpt, logger, 8*time.Second)
}

// NewAsynqServerWithShutdownTimeout is for services with long-running tasks
// that need a longer drain window. Most services should use NewAsynqServer.
func NewAsynqServerWithShutdownTimeout(redisClientOpt asynq.RedisClientOpt, logger *zap.Logger, shutdownTimeout time.Duration) *asynq.Server {
	logger = logger.Named("asynq.server")
	return asynq.NewServer(
		redisClientOpt,
		asynq.Config{
			Concurrency: 10,
			Queues: map[string]int{
				"critical":  6,
				"streaming": 6,
				"default":   3,
				"low":       1,
			},
			ShutdownTimeout: shutdownTimeout,
			Logger:          NewZapAsynqLogger(logger),
			IsFailure: func(err error) bool {
				if err != nil {
					return !strings.Contains(err.Error(), "retry after")
				}
				return false
			},
			RetryDelayFunc: func(n int, e error, t *asynq.Task) time.Duration {
				if e != nil {
					if strings.Contains(e.Error(), "retry after") {
						retryAfter := strings.Trim(strings.ReplaceAll(e.Error(), "retry after ", ""), " ")
						retryAfterInt, err := strconv.Atoi(retryAfter)
						if err == nil {
							return time.Duration(retryAfterInt) * time.Second
						}
					}
				}
				s := int(math.Pow(float64(n), 4)) + 15 + (rand.IntN(30) * (n + 1))
				return time.Duration(s) * time.Second
			},
		},
	)
}

func NewAsyncMonHandler(redisClientOpt asynq.RedisClientOpt) *asynqmon.HTTPHandler {
	return asynqmon.New(asynqmon.Options{
		RootPath:     "/monitoring",
		RedisConnOpt: redisClientOpt,
	})
}

func NewAsynqScheduler(redisClientOpt asynq.RedisClientOpt, logger *zap.Logger) (scheduler *asynq.Scheduler, err error) {
	scheduler = asynq.NewScheduler(redisClientOpt, &asynq.SchedulerOpts{
		Logger: logger.Sugar(),
	})
	return
}

func NewAsynqServerMux() (mux *asynq.ServeMux, err error) {
	mux = asynq.NewServeMux()
	return
}
