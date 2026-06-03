package services

import "context"

// PostgresDSN is a typed wrapper around the lib/pq DSN string consumed
// by pq.NewListener inside the outbox dispatcher. The wrapper exists so
// dig can disambiguate this string from any other bare `string`
// providers, and so the type lives in `services` (depended on by both
// `services/impl` and `injection`) — putting it in `injection` would
// create a `services/impl` → `injection` import cycle.
//
// The dig provider (injection.ProvidePostgresDSN) builds the DSN from
// the same db_config getters that the gorm pool uses, so the listener
// and the gorm connections always point at the same database.
type PostgresDSN string

// OutboxDispatcher is the long-running goroutine that drains
// sync_outbox into Asynq. It owns three responsibilities:
//
//  1. LISTEN/NOTIFY wakeup on the "sync_outbox_new" channel.
//  2. The three-state drain loop (pending → enqueuing → enqueued)
//     with FOR UPDATE SKIP LOCKED partitioning across replicas.
//  3. The reaper goroutine that resets rows stuck in `enqueuing` for
//     longer than the threshold.
//
// Run blocks until ctx is cancelled. WS6 wires this into the server
// lifecycle in cmd/server/main.go alongside the other long-lived
// background loops.
type OutboxDispatcher interface {
	Run(ctx context.Context) error
}
