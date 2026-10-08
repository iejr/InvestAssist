package db

import (
	"fmt"
	"log"
	"os"
	"time"

	"market-feed/internal/model"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// gormLogger raises the slow-SQL threshold to 1s so expected network latency
// doesn't spam warnings, while genuinely pathological queries still surface.
var gormLogger = logger.New(
	log.New(os.Stdout, "\r\n", log.LstdFlags),
	logger.Config{
		SlowThreshold: time.Second,
		LogLevel:      logger.Warn,
		Colorful:      true,
	},
)

// Open connects to Postgres and ensures price_candles exists. latest_prices
// has moved to Redis (see internal/latestcache); this is the only Postgres
// table market-feed owns now, so there is no primary/history split anymore.
func Open(dsn string) (*gorm.DB, error) {
	db, err := connect(dsn)
	if err != nil {
		return nil, fmt.Errorf("db: %w", err)
	}
	if err := db.AutoMigrate(&model.PriceCandle{}); err != nil {
		return nil, fmt.Errorf("migrate price_candles: %w", err)
	}
	log.Println("db: connected and migrated")
	return db, nil
}

func connect(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
}
