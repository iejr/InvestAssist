// Package db opens the Postgres connections and migrates the strategy tables.
package db

import (
	"fmt"
	"log"
	"os"
	"time"

	"strategy-server-go/internal/model"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var gormLogger = logger.New(
	log.New(os.Stdout, "\r\n", log.LstdFlags),
	logger.Config{
		SlowThreshold: time.Second,
		LogLevel:      logger.Warn,
		Colorful:      true,
	},
)

// Open connects to Postgres and migrates ONLY the strategy-owned tables
// (strategies, transactions). price_candles is owned and migrated by
// market-feed; latest_prices lives in Redis — neither is touched here.
func Open(dsn string) (*gorm.DB, error) {
	db, err := connect(dsn)
	if err != nil {
		return nil, fmt.Errorf("db: %w", err)
	}
	if err := db.AutoMigrate(&model.Strategy{}, &model.Transaction{}); err != nil {
		return nil, fmt.Errorf("migrate strategy tables: %w", err)
	}
	log.Println("db: connected and migrated")
	return db, nil
}

func connect(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormLogger})
}
