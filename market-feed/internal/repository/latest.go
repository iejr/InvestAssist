package repository

import (
	"market-feed/internal/model"
	"market-feed/internal/provider"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LatestRepo upserts the newest observed value for each (base, quote) edge.
type LatestRepo struct {
	db *gorm.DB
}

func NewLatestRepo(db *gorm.DB) *LatestRepo { return &LatestRepo{db: db} }

// Upsert writes the event as the latest price for its edge, overwriting any
// existing row for the same (base, quote).
func (r *LatestRepo) Upsert(ev provider.PriceEvent) error {
	row := model.LatestPrice{
		Base:      ev.Base,
		Quote:     ev.Quote,
		Price:     ev.Price,
		Source:    ev.Source,
		UpdatedAt: ev.Timestamp,
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "base"}, {Name: "quote"}},
		DoUpdates: clause.AssignmentColumns([]string{"price", "source", "updated_at"}),
	}).Create(&row).Error
}
