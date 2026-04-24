package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type StrategyType string

const (
	StrategyTypeDCA StrategyType = "DCA"
	StrategyTypeVA  StrategyType = "VA"
)

type Strategy struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	Name      string         `json:"name"`
	Type      StrategyType   `json:"type"`
	Security  string         `json:"security"`
	StartDate time.Time      `json:"start_date"`
	Interval  string         `json:"interval"` // e.g., "weekly", "monthly"
	Increment float64        `json:"increment"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type TransactionType string

const (
	TransactionTypeBuy  TransactionType = "BUY"
	TransactionTypeSell TransactionType = "SELL"
)

type Transaction struct {
	ID         uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	StrategyID uuid.UUID       `gorm:"type:uuid;index" json:"strategy_id"`
	Type       TransactionType `json:"type"`
	Security   string          `json:"security"`
	Shares     float64         `json:"shares"`
	Price      float64         `json:"price"`
	Timestamp  time.Time       `json:"timestamp"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

type PriceHistory struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Security  string    `gorm:"index:idx_security_date" json:"security"`
	Date      time.Time `gorm:"index:idx_security_date" json:"date"`
	Price     float64   `json:"price"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Strategy) BeforeCreate(tx *gorm.DB) (err error) {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return
}

func (t *Transaction) BeforeCreate(tx *gorm.DB) (err error) {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return
}
