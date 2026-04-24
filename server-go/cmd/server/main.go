package main

import (
	"invest-assist-go/internal/api"
	"invest-assist-go/internal/db"
	"invest-assist-go/pkg/worker"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	// Initialize database
	db.InitDB()

	// Initialize Echo
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Register Handlers
	api.RegisterHandlers(e)

	// Start Price Worker
	worker.StartPriceWorker()

	// Start server
	e.Logger.Fatal(e.Start(":3000"))
}
