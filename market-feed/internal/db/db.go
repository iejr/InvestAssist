package db

import (
	"fmt"
	"log"

	"market-feed/internal/model"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open connects to Postgres and ensures the market-feed tables exist.
// AutoMigrate is used for convenience in development; the migrations/ folder
// holds the authoritative SQL for production/CI.
func Open(dsn string) (*gorm.DB, error) {
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	if err := gdb.AutoMigrate(&model.LatestPrice{}, &model.PriceCandle{}); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	log.Println("db: connected and migrated")
	return gdb, nil
}
