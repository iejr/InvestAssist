package repository

import (
	"market-feed/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CandleRepo appends OHLC observations. Writes are idempotent on the
// (base, quote, interval, open_time, source) unique key so a re-emitted or
// backfilled candle does not duplicate.
type CandleRepo struct {
	db *gorm.DB
}

func NewCandleRepo(db *gorm.DB) *CandleRepo { return &CandleRepo{db: db} }

// Insert appends a candle, ignoring the write if that bucket already exists.
func (r *CandleRepo) Insert(c model.PriceCandle) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "base"}, {Name: "quote"}, {Name: "interval"},
			{Name: "open_time"}, {Name: "source"},
		},
		DoNothing: true,
	}).Create(&c).Error
}
