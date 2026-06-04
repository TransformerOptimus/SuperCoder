package services

import "github.com/redis/go-redis/v9"

// StreamContentRedisClient is a typed wrapper around *redis.Client used
// by /stream (services/impl/sync_session_service_ingest.go) to write
// per-batch file content with a 2-hour TTL. The wrapper exists for two
// reasons:
//
//  1. dig needs a distinct type to disambiguate this client from the
//     existing cacher_client.Caches (which targets CacheDB and locks
//     the TTL to a config value).
//
//  2. The type lives in the `services` package — not in `injection` —
//     because `services/impl` and `injection` both depend on `services`,
//     so a typed wrapper here avoids a `services/impl` → `injection`
//     back-edge that would form an import cycle.
//
// IMPORTANT: the dig provider (injection.ProvideStreamContentRedisClient)
// pins this client to RedisConfig.WorkerDB(), the SAME logical Redis DB
// Asynq uses (pkg/config/asynq/asynq.go:12). The /stream writer (the
// API process) and the /stream_batch readers (the Asynq workers in WS5)
// MUST hit the same DB or content keys will be invisible to workers.
//
// Keys are namespaced "supercoder:sync:{sync_id}:batch:{batch_id}:content"
// — service prefix included so the key cannot collide with Asynq's own
// keys or any other worker payload that lands in WorkerDB.
type StreamContentRedisClient struct {
	*redis.Client
}
