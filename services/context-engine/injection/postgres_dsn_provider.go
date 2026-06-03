package injection

import (
	"fmt"

	dbconfig "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/config/db"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// ProvidePostgresDSN is the dig provider for services.PostgresDSN. It
// builds the DSN from the same db_config getters that
// pkg/clients/postgres/postgres.go:18 uses to open the gorm pool, so
// the listener (in the outbox dispatcher) and the gorm connections
// always point at the same database. Format matches lib/pq's
// connection-string parser (`host= user= password= dbname= port=
// sslmode=`).
//
// SSL handling: lib/pq's default sslmode is `prefer`, which silently
// falls back to plaintext if the server rejects TLS. That is a
// security downgrade for any RDS-style deployment that mandates TLS,
// so we ALWAYS emit an explicit sslmode — `require` when SSL is
// enabled (encrypts but doesn't verify the cert chain, matching the
// existing gorm pool's pgx defaults via gorm.io/driver/postgres) and
// `disable` otherwise. If a deployment needs `verify-full`, surface
// it through db_config rather than removing the explicit `require`.
//
// The named type lives in the `services` package (not here) to avoid
// a services/impl → injection import cycle.
func ProvidePostgresDSN(cfg dbconfig.DBConfig) services.PostgresDSN {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s",
		cfg.Host(),
		cfg.User(),
		cfg.Password(),
		cfg.DBName(),
		cfg.Port(),
	)
	if cfg.IsSSL() {
		dsn += " sslmode=require"
	} else {
		dsn += " sslmode=disable"
	}
	return services.PostgresDSN(dsn)
}
