package db

import (
	"fmt"
	"log"
	"os"

	"invest-assist-go/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		// Default for local development if not provided
		dsn = "host=localhost user=postgres password=postgres dbname=invest_assist port=5432 sslmode=disable"
	}

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	fmt.Println("Database connection established")

	// Auto-migrate models
	err = DB.AutoMigrate(&model.Strategy{}, &model.Transaction{}, &model.PriceHistory{})
	if err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	fmt.Println("Database migration completed")
}
