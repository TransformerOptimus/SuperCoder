package impl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/lib/pq"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/db/postgres"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// Dispatcher tunables. Plan §8.4 / §3.14 v3.1. These are constants
// rather than config values because there is no realistic operational
// reason to vary them per-deployment — the dispatcher is correct at
// these settings and any change should come with a code review.
const (
	// claimBatchSize bounds how many outbox rows we drain in one
	// ClaimPending call. The drain loop loops until ClaimPending
	// returns 0, so this is purely a memory/latency tradeoff.
	claimBatchSize = 100

	// reaperInterval is how often the reaper checks for stuck rows.
	reaperInterval = 60 * time.Second

	// reaperThreshold is the age past which an `enqueuing` row is
	// considered abandoned and reset to `pending`.
	reaperThreshold = 5 * time.Minute

	// pollFallback is the polling tick that backs up LISTEN/NOTIFY.
	// If a NOTIFY is missed (process crash between commit and notify,
	// or listener-reconnect window) the dispatcher still picks up new
	// work within this interval.
	pollFallback = 1 * time.Second

	// listenerMinReconnect / listenerMaxReconnect bracket the
	// pq.NewListener reconnect backoff.
	listenerMinReconnect = 10 * time.Second
	listenerMaxReconnect = time.Minute

	// streamBatchTaskTimeout / finalizeTaskTimeout cap how long an
	// individual Asynq task may run before Asynq considers it failed.
	// Per-batch indexing is bounded by the embed semaphore + max
	// batch size; the finalizer is bounded by Merkle CAS retry loops.
	streamBatchTaskTimeout = 5 * time.Minute
	finalizeTaskTimeout    = 10 * time.Minute
)

// asynqEnqueuer is the test seam for OutboxDispatcher. The real impl
// passes *asynq.Client; tests pass an in-memory recorder. The interface
// is the smallest subset of *asynq.Client we depend on.
type asynqEnqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// pgListener is the test seam for pq.NewListener. Production wires the
// real lib/pq listener; tests pass an in-process implementation that
// fires a Notify channel on demand.
type pgListener interface {
	Listen(channel string) error
	Close() error
	NotifyChannel() <-chan *pq.Notification
}

// pqListenerWrapper adapts *pq.Listener to pgListener. The real
// listener exposes Notify as a struct field; the wrapper turns it into
// a method so the interface stays small.
type pqListenerWrapper struct {
	*pq.Listener
}

func (w pqListenerWrapper) NotifyChannel() <-chan *pq.Notification { return w.Listener.Notify }

type outboxDispatcherImpl struct {
	logger      *zap.Logger
	db          *gorm.DB
	dbURL       string
	outboxRepo  repositories.SyncOutboxRepository
	asynqClient asynqEnqueuer

	// listenerFactory is overridable in tests so we don't need a real
	// pq.NewListener (which talks to Postgres). Production calls
	// newPqListener.
	listenerFactory func(dbURL string, logger *zap.Logger) (pgListener, error)
}

// NewOutboxDispatcher is the dig provider. Production callers receive
// a dispatcher backed by a real *asynq.Client and pq.NewListener.
func NewOutboxDispatcher(
	logger *zap.Logger,
	db *gorm.DB,
	dsn services.PostgresDSN,
	outboxRepo repositories.SyncOutboxRepository,
	asynqClient *asynq.Client,
) services.OutboxDispatcher {
	return &outboxDispatcherImpl{
		logger:          logger.Named("services.outbox_dispatcher"),
		db:              db,
		dbURL:           string(dsn),
		outboxRepo:      outboxRepo,
		asynqClient:     asynqClient,
		listenerFactory: newPqListener,
	}
}

// newPqListener is the production listener factory. The 10 s / 60 s
// values bracket how aggressively pq retries the underlying TCP
// connection on failure.
func newPqListener(dbURL string, logger *zap.Logger) (pgListener, error) {
	listener := pq.NewListener(dbURL, listenerMinReconnect, listenerMaxReconnect, func(ev pq.ListenerEventType, err error) {
		if err != nil {
			logger.Error("pq listener event", zap.Int("event", int(ev)), zap.Error(err))
		}
	})
	return pqListenerWrapper{Listener: listener}, nil
}

// Run is the dispatcher's main loop. It blocks until ctx is cancelled.
// The select serves three event sources:
//
//	ctx.Done()         → graceful shutdown
//	listener.Notify    → wake up on a fresh /stream commit
//	pollTicker.C       → 1 s polling fallback for missed NOTIFYs
//
// The reaper runs in a separate goroutine on its own ticker so a stuck
// drainPending call can't starve it.
func (d *outboxDispatcherImpl) Run(ctx context.Context) error {
	listener, err := d.listenerFactory(d.dbURL, d.logger)
	if err != nil {
		return fmt.Errorf("create listener: %w", err)
	}
	defer func() { _ = listener.Close() }()
	if err := listener.Listen("sync_outbox_new"); err != nil {
		return fmt.Errorf("listen sync_outbox_new: %w", err)
	}

	go d.runReaper(ctx)

	// Drain once on startup so any rows that landed before this
	// dispatcher came up don't sit until the first NOTIFY/poll tick.
	d.drainPending(ctx)

	pollTicker := time.NewTicker(pollFallback)
	defer pollTicker.Stop()

	notifyCh := listener.NotifyChannel()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-notifyCh:
			d.drainPending(ctx)
		case <-pollTicker.C:
			d.drainPending(ctx)
		}
	}
}

// drainPending runs the three-state transition until the outbox is
// empty:
//
//	Phase 1: ClaimPending atomically flips up to N rows pending → enqueuing
//	         (single-statement CTE; no DB row locks held after it returns).
//	Phase 2: Enqueue each claimed row to Asynq, partitioned ok / fail.
//	Phase 3: MarkEnqueued the OKs; ResetToPending the failures.
//
// CRITICAL: Phase 2 runs WITHOUT any DB locks held. ClaimPending's
// transaction commits before returning, so the per-row Asynq round-trip
// (which can take seconds) does not block other dispatchers from
// claiming the next batch. The
// TestDispatcher_DoesNotHoldDBLocksDuringAsynqIO regression test asserts
// this directly.
func (d *outboxDispatcherImpl) drainPending(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		rows, err := d.outboxRepo.ClaimPending(ctx, claimBatchSize)
		if err != nil {
			d.logger.Error("claim pending failed", zap.Error(err))
			return
		}
		if len(rows) == 0 {
			return
		}

		var ok, fail []int64
		for _, r := range rows {
			if err := d.enqueueOne(ctx, r); err != nil {
				d.logger.Error("enqueue task failed",
					zap.Error(err),
					zap.Int64("row_id", r.ID),
					zap.String("task_type", string(r.TaskType)),
					zap.String("sync_id", r.SyncID),
				)
				fail = append(fail, r.ID)
			} else {
				ok = append(ok, r.ID)
			}
		}

		if len(ok) > 0 {
			if err := d.outboxRepo.MarkEnqueued(ctx, ok); err != nil {
				// Rows stay in `enqueuing`; the reaper will reset them
				// after reaperThreshold. This is expected and recoverable
				// — log so it's visible but don't crash.
				d.logger.Error("mark enqueued failed (reaper will rescue)",
					zap.Error(err),
					zap.Int("count", len(ok)),
				)
			}
		}
		if len(fail) > 0 {
			if err := d.outboxRepo.ResetToPending(ctx, fail); err != nil {
				d.logger.Error("reset to pending failed", zap.Error(err), zap.Int("count", len(fail)))
			}
		}
	}
}

// enqueueOne dispatches a single outbox row to Asynq. Errors propagate
// up to drainPending which decides between MarkEnqueued (success) and
// ResetToPending (failure).
//
// asynq.ErrTaskIDConflict is treated as success: the deterministic task
// ID means a duplicate enqueue is the intended idempotent behaviour
// when a row is reclaimed after a transient Phase 3 failure.
func (d *outboxDispatcherImpl) enqueueOne(ctx context.Context, r *postgres.SyncOutbox) error {
	var (
		task    *asynq.Task
		taskID  string
		timeout time.Duration
	)

	switch r.TaskType {
	case postgres.OutboxTaskStreamBatch:
		payload, err := json.Marshal(services.StreamBatchPayload{
			SyncID:   r.SyncID,
			BatchID:  r.BatchID,
			RedisKey: r.RedisKey,
		})
		if err != nil {
			return fmt.Errorf("marshal stream_batch payload: %w", err)
		}
		task = asynq.NewTask(services.TaskTypeStreamBatch, payload)
		taskID = services.StreamBatchTaskID(r.SyncID, r.BatchID)
		timeout = streamBatchTaskTimeout

	case postgres.OutboxTaskFinalize:
		payload, err := json.Marshal(services.FinalizePayload{SyncID: r.SyncID})
		if err != nil {
			return fmt.Errorf("marshal finalize payload: %w", err)
		}
		task = asynq.NewTask(services.TaskTypeFinalize, payload)
		taskID = services.FinalizeTaskID(r.SyncID)
		timeout = finalizeTaskTimeout

	default:
		return fmt.Errorf("unknown task_type %q", r.TaskType)
	}

	// Route stream_batch + finalize:sync onto the dedicated "streaming"
	// queue so they aren't starved by long-running codereview:* tasks
	// sharing the "default" queue. Queue is registered at weight=6 in
	// pkg/clients/asynq/asynq.go — equal weight to "critical".
	_, err := d.asynqClient.EnqueueContext(ctx, task,
		asynq.TaskID(taskID),
		asynq.MaxRetry(3),
		asynq.Timeout(timeout),
		asynq.Queue("streaming"),
	)
	if err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
		return err
	}
	return nil
}

// runReaper resets any row stuck in `enqueuing` for longer than
// reaperThreshold back to `pending` so another dispatcher can pick it
// up. Stuckness is detected via claimed_at on the row.
func (d *outboxDispatcherImpl) runReaper(ctx context.Context) {
	ticker := time.NewTicker(reaperInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := d.outboxRepo.ReapStuck(ctx, reaperThreshold)
			if err != nil {
				d.logger.Error("reaper failed", zap.Error(err))
				continue
			}
			if n > 0 {
				d.logger.Warn("reaper reset stuck outbox rows", zap.Int64("count", n))
			}
		}
	}
}
