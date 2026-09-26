// Command server runs the strategy HTTP API. It reads market data from the
// shared market-feed tables (no mock price worker) and serves VA/DCA valuations.
package main

import (
	"log"

	"strategy-server-go/internal/api"
	"strategy-server-go/internal/config"
	"strategy-server-go/internal/db"
	"strategy-server-go/internal/pricing"
	"strategy-server-go/internal/strategy"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	cfg := config.Load()

	conns, err := db.Open(cfg.DatabaseURL, cfg.HistoryDatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	pricer := pricing.New(conns.Primary, conns.History)
	svc := strategy.New(conns.Primary, pricer, cfg.MaxStalenessOverride)
	h := api.New(conns.Primary, pricer, svc, nil)

	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	h.Register(e)

	log.Printf("strategy-server listening on :%s", cfg.Port)
	e.Logger.Fatal(e.Start(":" + cfg.Port))
}
