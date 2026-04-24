package service

import (
	"invest-assist-go/internal/db"
	"invest-assist-go/internal/model"
	"time"

	"github.com/google/uuid"
)

type HistoryPoint struct {
	Date          time.Time `json:"date"`
	ActualValue   float64   `json:"actual_value"`
	ExpectedValue float64   `json:"expected_value"`
}

type StrategySummary struct {
	CurrentValue      float64 `json:"current_value"`
	TotalCashInvested float64 `json:"total_cash_invested"`
	GainLoss          float64 `json:"gain_loss"`
	NextInvestment    float64 `json:"next_investment"`
}

func GetHistory(strategyID uuid.UUID) ([]HistoryPoint, error) {
	var strategy model.Strategy
	if err := db.DB.First(&strategy, "id = ?", strategyID).Error; err != nil {
		return nil, err
	}

	var transactions []model.Transaction
	if err := db.DB.Find(&transactions, "strategy_id = ?", strategyID).Order("timestamp asc").Error; err != nil {
		return nil, err
	}

	// For simplicity in MVP, we fetch all price history for the security
	var prices []model.PriceHistory
	if err := db.DB.Find(&prices, "security = ?", strategy.Security).Order("date asc").Error; err != nil {
		return nil, err
	}

	dateSeq := generateDateSequence(strategy.StartDate, strategy.Interval)
	history := make([]HistoryPoint, 0)

	var currentShares float64
	var totalCost float64
	txIdx := 0

	for _, date := range dateSeq {
		// Process transactions up to this date
		for txIdx < len(transactions) && !transactions[txIdx].Timestamp.After(date) {
			t := transactions[txIdx]
			if t.Type == model.TransactionTypeBuy {
				currentShares += t.Shares
				totalCost += t.Shares * t.Price
			} else {
				currentShares -= t.Shares
				totalCost -= t.Shares * t.Price // Simple cost reduction for MVP
			}
			txIdx++
		}

		// Find price for this date (or closest previous)
		price := getPriceForDate(prices, date)
		actualValue := currentShares * price

		var expectedValue float64
		if strategy.Type == model.StrategyTypeVA {
			// VA: Expected value grows by increment each interval
			monthsSinceStart := getIntervalCount(strategy.StartDate, date, strategy.Interval)
			expectedValue = float64(monthsSinceStart) * strategy.Increment
		} else {
			// DCA: Expected value is total invested amount (simplified)
			monthsSinceStart := getIntervalCount(strategy.StartDate, date, strategy.Interval)
			expectedValue = float64(monthsSinceStart) * strategy.Increment
		}

		history = append(history, HistoryPoint{
			Date:          date,
			ActualValue:   actualValue,
			ExpectedValue: expectedValue,
		})
	}

	return history, nil
}

func GetSummary(strategyID uuid.UUID) (*StrategySummary, error) {
	history, err := GetHistory(strategyID)
	if err != nil {
		return nil, err
	}

	if len(history) == 0 {
		return &StrategySummary{}, nil
	}

	lastPoint := history[len(history)-1]
	
	// Calculate total cash invested from transactions
	var transactions []model.Transaction
	db.DB.Find(&transactions, "strategy_id = ?", strategyID)
	
	var totalInvested float64
	for _, t := range transactions {
		if t.Type == model.TransactionTypeBuy {
			totalInvested += t.Shares * t.Price
		} else {
			totalInvested -= t.Shares * t.Price
		}
	}

	nextInvestment := 0.0
	var strategy model.Strategy
	db.DB.First(&strategy, "id = ?", strategyID)

	if strategy.Type == model.StrategyTypeVA {
		nextExpected := lastPoint.ExpectedValue + strategy.Increment
		nextInvestment = nextExpected - lastPoint.ActualValue
	} else {
		nextInvestment = strategy.Increment
	}

	return &StrategySummary{
		CurrentValue:      lastPoint.ActualValue,
		TotalCashInvested: totalInvested,
		GainLoss:          lastPoint.ActualValue - totalInvested,
		NextInvestment:    nextInvestment,
	}, nil
}

// Helper functions

func generateDateSequence(start time.Time, interval string) []time.Time {
	sequence := make([]time.Time, 0)
	current := start
	now := time.Now()

	for !current.After(now) {
		sequence = append(sequence, current)
		if interval == "weekly" {
			current = current.AddDate(0, 0, 7)
		} else {
			current = current.AddDate(0, 1, 0)
		}
	}
	return sequence
}

func getPriceForDate(prices []model.PriceHistory, date time.Time) float64 {
	var lastPrice float64
	for _, p := range prices {
		if p.Date.After(date) {
			break
		}
		lastPrice = p.Price
	}
	return lastPrice
}

func getIntervalCount(start, end time.Time, interval string) int {
	if interval == "weekly" {
		days := end.Sub(start).Hours() / 24
		return int(days/7) + 1
	}
	// Monthly
	years := end.Year() - start.Year()
	months := int(end.Month()) - int(start.Month())
	return years*12 + months + 1
}
