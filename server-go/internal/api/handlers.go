package api

import (
	"invest-assist-go/internal/db"
	"invest-assist-go/internal/model"
	"invest-assist-go/internal/service"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func RegisterHandlers(e *echo.Echo) {
	e.GET("/strategy", listStrategies)
	e.POST("/strategy", createStrategy)
	e.GET("/history/:id", getHistory)
	e.GET("/summary/:id", getSummary)
	e.POST("/transaction", createTransaction)
	e.GET("/securities", listSecurities)
}

func listStrategies(c echo.Context) error {
	var strategies []model.Strategy
	db.DB.Find(&strategies)
	return c.JSON(http.StatusOK, strategies)
}

func createStrategy(c echo.Context) error {
	s := new(model.Strategy)
	if err := c.Bind(s); err != nil {
		return err
	}
	db.DB.Create(s)
	return c.JSON(http.StatusCreated, s)
}

func getHistory(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}
	history, err := service.GetHistory(id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, history)
}

func getSummary(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}
	summary, err := service.GetSummary(id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, summary)
}

func createTransaction(c echo.Context) error {
	t := new(model.Transaction)
	if err := c.Bind(t); err != nil {
		return err
	}
	db.DB.Create(t)
	return c.JSON(http.StatusCreated, t)
}

func listSecurities(c echo.Context) error {
	// For MVP, return a fixed list
	securities := []string{"BTC", "ETH", "SPY"}
	return c.JSON(http.StatusOK, securities)
}
