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

// Conns holds the database handles. Primary owns the strategy tables and
// latest_prices; History holds price_candles and may point at a separate
// instance (or at Primary when no split is configured), mirroring market-feed.
type Conns struct {
	Primary *gorm.DB
	History *gorm.DB
}

// Open connects to Postgres and migrates ONLY the strategy-owned tables. The
// market-feed tables (latest_prices, price_candles) are owned and migrated by
// market-feed; this service never touches their schema.
func Open(primaryDSN, historyDSN string) (*Conns, error) {
	primary, err := connect(primaryDSN)
	if err != nil {
		return nil, fmt.Errorf("primary db: %w", err)
	}
	if err := primary.AutoMigrate(&model.Strategy{}, &model.Transaction{}); err != nil {
		return nil, fmt.Errorf("migrate strategy tables: %w", err)
	}

	history := primary
	if historyDSN != "" {
		history, err = connect(historyDSN)
		if err != nil {
			return nil, fmt.Errorf("history db: %w", err)
		}
		log.Println("db: price_candles read from separate MF_HISTORY_DATABASE_URL")
	} else {
		log.Println("db: price_candles read from the primary database")
	}

	log.Println("db: connected and migrated")
	return &Conns{Primary: primary, History: history}, nil
}

func connect(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormLogger})
}
