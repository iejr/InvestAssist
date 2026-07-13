package db

import (
	"fmt"
	"log"

	"market-feed/internal/model"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Conns holds the database handles used by the service. Primary always holds
// latest_prices; History holds price_candles and may point at a separate
// instance (or at Primary when no split is configured).
type Conns struct {
	Primary *gorm.DB
	History *gorm.DB
}

// Open connects to Postgres and ensures the market-feed tables exist.
//
// primaryDSN holds latest_prices. If historyDSN is non-empty it is opened as a
// separate connection for price_candles; otherwise history shares Primary.
// AutoMigrate is used for convenience in development; the migrations/ folder
// holds the authoritative SQL for production/CI.
func Open(primaryDSN, historyDSN string) (*Conns, error) {
	primary, err := connect(primaryDSN)
	if err != nil {
		return nil, fmt.Errorf("primary db: %w", err)
	}
	if err := primary.AutoMigrate(&model.LatestPrice{}); err != nil {
		return nil, fmt.Errorf("migrate latest_prices: %w", err)
	}

	history := primary
	if historyDSN != "" {
		// A configured-but-unreachable history DB is a hard error: silently
		// falling back to Primary is exactly the confusion we want to avoid.
		history, err = connect(historyDSN)
		if err != nil {
			return nil, fmt.Errorf("history db: %w", err)
		}
		log.Println("db: history (price_candles) uses separate MF_HISTORY_DATABASE_URL")
	} else {
		log.Println("db: history (price_candles) shares the primary database")
	}
	if err := history.AutoMigrate(&model.PriceCandle{}); err != nil {
		return nil, fmt.Errorf("migrate price_candles: %w", err)
	}

	log.Println("db: connected and migrated")
	return &Conns{Primary: primary, History: history}, nil
}

func connect(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
}
