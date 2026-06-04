package postgres_client

import (
	"fmt"

	"github.com/go-gorm/caches/v4"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/uptrace/opentelemetry-go-extra/otelgorm"

	db_config "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/config/db"
)

func NewPostgresDb(dbConfig db_config.DBConfig, cache *caches.Caches, logger *zap.Logger, gormLogger logger.Interface) (db *gorm.DB, err error) {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s",
		dbConfig.Host(),
		dbConfig.User(),
		dbConfig.Password(),
		dbConfig.DBName(),
		dbConfig.Port(),
	)

	if !dbConfig.IsSSL() {
		dsn += " sslmode=disable"
	}

	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		logger.Error("could not connect to the postgres database", zap.Error(err))
		return nil, err
	}

	if err := db.Use(otelgorm.NewPlugin()); err != nil {
		logger.Error("could not use otelgorm", zap.Error(err))
		return nil, err
	}

	// Verify connection
	sqlDB, err := db.DB()
	if err != nil {
		logger.Error("could not get database instance", zap.Error(err))
		return nil, err
	}

	err = sqlDB.Ping()
	if err != nil {
		logger.Error("could not ping the postgres database", zap.Error(err))
		return nil, err
	}

	err = db.Use(cache)
	if err != nil {
		logger.Error("could not use cache", zap.Error(err))
		return nil, err
	}
	logger.Info("Connected to database", zap.String("name", dbConfig.DBName()))
	return
}

func NewPostgresDbWithoutCache(dbConfig db_config.DBConfig, logger *zap.Logger, gormLogger logger.Interface) (db *gorm.DB, err error) {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s",
		dbConfig.Host(),
		dbConfig.User(),
		dbConfig.Password(),
		dbConfig.DBName(),
		dbConfig.Port(),
	)

	if !dbConfig.IsSSL() {
		dsn += " sslmode=disable"
	}

	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		logger.Error("could not connect to the postgres database", zap.Error(err))
		return nil, err
	}

	if err := db.Use(otelgorm.NewPlugin()); err != nil {
		logger.Error("could not use otelgorm", zap.Error(err))
		return nil, err
	}

	// Verify connection
	sqlDB, err := db.DB()
	if err != nil {
		logger.Error("could not get database instance", zap.Error(err))
		return nil, err
	}

	err = sqlDB.Ping()
	if err != nil {
		logger.Error("could not ping the postgres database", zap.Error(err))
		return nil, err
	}

	logger.Info("Connected to database", zap.String("name", dbConfig.DBName()))
	return
}
