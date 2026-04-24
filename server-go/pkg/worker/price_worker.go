package worker

import (
	"invest-assist-go/internal/db"
	"invest-assist-go/internal/model"
	"log"
	"math/rand"
	"time"
)

func StartPriceWorker() {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for range ticker.C {
			fetchAndStorePrices()
		}
	}()
	
	// Also run once on startup
	fetchAndStorePrices()
}

func fetchAndStorePrices() {
	log.Println("Fetching prices...")
	securities := []string{"BTC", "ETH", "SPY"}
	
	for _, s := range securities {
		// In a real app, call an API here.
		// For MVP, generate a random price or use a mock.
		price := generateMockPrice(s)
		
		priceHistory := model.PriceHistory{
			Security: s,
			Date:     time.Now().Truncate(24 * time.Hour),
			Price:    price,
		}
		
		// Check if price for today already exists
		var existing model.PriceHistory
		result := db.DB.Where("security = ? AND date = ?", s, priceHistory.Date).First(&existing)
		if result.Error != nil {
			db.DB.Create(&priceHistory)
		} else {
			existing.Price = price
			db.DB.Save(&existing)
		}
	}
}

func generateMockPrice(security string) float64 {
	switch security {
	case "BTC":
		return 40000 + rand.Float64()*20000
	case "ETH":
		return 2000 + rand.Float64()*1000
	default:
		return 400 + rand.Float64()*100
	}
}
