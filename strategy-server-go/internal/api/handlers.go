// Package api exposes the HTTP surface (Echo). It reads market data via pricing
// and computes valuations via the strategy service; it never fabricates prices.
package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"strategy-server-go/internal/model"
	"strategy-server-go/internal/pricing"
	"strategy-server-go/internal/strategy"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// Handler bundles the dependencies the routes need.
type Handler struct {
	db     *gorm.DB
	pricer *pricing.Pricer
	svc    *strategy.Service
	now    func() time.Time
}

// New builds a Handler. now is injectable for tests; nil ⇒ time.Now.
func New(db *gorm.DB, pricer *pricing.Pricer, svc *strategy.Service, now func() time.Time) *Handler {
	if now == nil {
		now = time.Now
	}
	return &Handler{db: db, pricer: pricer, svc: svc, now: now}
}

// Register wires the routes onto e.
func (h *Handler) Register(e *echo.Echo) {
	e.GET("/bases", h.listBases)
	e.GET("/quotes", h.listQuotes)

	e.GET("/strategies", h.listStrategies)
	e.POST("/strategies", h.createStrategy)
	e.GET("/strategies/:id", h.getStrategy)
	e.GET("/strategies/:id/history", h.getHistory)
	e.GET("/strategies/:id/summary", h.getSummary)
	e.GET("/strategies/:id/display-currencies", h.getDisplayCurrencies)

	e.GET("/transactions", h.listTransactions)
	e.POST("/transactions", h.createTransaction)
}

func (h *Handler) listBases(c echo.Context) error {
	bases, err := h.pricer.Bases()
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, bases)
}

// listQuotes returns the currencies reachable from ?base= — the valid quote
// options for a new strategy on that base.
func (h *Handler) listQuotes(c echo.Context) error {
	base := strings.TrimSpace(c.QueryParam("base"))
	if base == "" {
		return badRequest(c, "base query param required")
	}
	quotes, err := h.pricer.ReachableQuotes(base)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, quotes)
}

func (h *Handler) listStrategies(c echo.Context) error {
	var strategies []model.Strategy
	if err := h.db.Order("created_at desc").Find(&strategies).Error; err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, strategies)
}

// createStrategyReq is the create-strategy body. start_date accepts a plain
// date (YYYY-MM-DD) or RFC3339 timestamp.
type createStrategyReq struct {
	Name      string             `json:"name"`
	Type      model.StrategyType `json:"type"`
	Base      string             `json:"base"`
	Quote     string             `json:"quote"`
	StartDate string             `json:"start_date"`
	Interval  model.Interval     `json:"interval"`
	Increment float64            `json:"increment"`
}

func (h *Handler) createStrategy(c echo.Context) error {
	var req createStrategyReq
	if err := c.Bind(&req); err != nil {
		return badRequest(c, "invalid body")
	}
	if req.Name == "" || req.Base == "" || req.Quote == "" {
		return badRequest(c, "name, base and quote are required")
	}
	if req.Type != model.StrategyTypeVA && req.Type != model.StrategyTypeDCA {
		return badRequest(c, "type must be VA or DCA")
	}
	if req.Interval != model.IntervalWeekly && req.Interval != model.IntervalMonthly {
		return badRequest(c, "interval must be weekly or monthly")
	}
	start, err := parseDate(req.StartDate)
	if err != nil {
		return badRequest(c, "invalid start_date")
	}

	// Validate the (base, quote) is actually priceable from the feed today.
	quotes, err := h.pricer.ReachableQuotes(req.Base)
	if err != nil {
		return serverErr(c, err)
	}
	if !contains(quotes, req.Quote) && req.Quote != req.Base {
		return badRequest(c, "quote is not reachable from base in the feed")
	}

	s := model.Strategy{
		Name:      req.Name,
		Type:      req.Type,
		Base:      req.Base,
		Quote:     req.Quote,
		StartDate: start,
		Interval:  req.Interval,
		Increment: req.Increment,
	}
	if err := h.db.Create(&s).Error; err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusCreated, s)
}

func (h *Handler) getStrategy(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return badRequest(c, "invalid id")
	}
	var s model.Strategy
	if err := h.db.First(&s, "id = ?", id).Error; err != nil {
		return notFoundOrErr(c, err)
	}
	return c.JSON(http.StatusOK, s)
}

func (h *Handler) getHistory(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return badRequest(c, "invalid id")
	}
	display := strings.TrimSpace(c.QueryParam("display"))
	now := h.now()
	rangeFrom := parseRange(c.QueryParam("range"), now)

	hist, err := h.svc.GetHistory(id, display, rangeFrom, now)
	if err != nil {
		return notFoundOrErr(c, err)
	}
	return c.JSON(http.StatusOK, hist)
}

func (h *Handler) getSummary(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return badRequest(c, "invalid id")
	}
	display := strings.TrimSpace(c.QueryParam("display"))
	sum, err := h.svc.GetSummary(id, display, h.now())
	if err != nil {
		return notFoundOrErr(c, err)
	}
	return c.JSON(http.StatusOK, sum)
}

func (h *Handler) getDisplayCurrencies(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return badRequest(c, "invalid id")
	}
	options, def, err := h.svc.DisplayCurrencies(id)
	if err != nil {
		return notFoundOrErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"options": options, "default": def})
}

func (h *Handler) listTransactions(c echo.Context) error {
	sid := strings.TrimSpace(c.QueryParam("strategy_id"))
	q := h.db.Order("timestamp asc")
	if sid != "" {
		id, err := uuid.Parse(sid)
		if err != nil {
			return badRequest(c, "invalid strategy_id")
		}
		q = q.Where("strategy_id = ?", id)
	}
	var txs []model.Transaction
	if err := q.Find(&txs).Error; err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, txs)
}

// createTransactionReq is the record-transaction body.
type createTransactionReq struct {
	StrategyID  string                `json:"strategy_id"`
	Type        model.TransactionType `json:"type"`
	Shares      float64               `json:"shares"`
	Price       float64               `json:"price"`
	FeeCurrency string                `json:"fee_currency"`
	FeeAmount   float64               `json:"fee_amount"`
	Timestamp   string                `json:"timestamp"`
}

func (h *Handler) createTransaction(c echo.Context) error {
	var req createTransactionReq
	if err := c.Bind(&req); err != nil {
		return badRequest(c, "invalid body")
	}
	sid, err := uuid.Parse(req.StrategyID)
	if err != nil {
		return badRequest(c, "invalid strategy_id")
	}
	if req.Type != model.TransactionTypeBuy && req.Type != model.TransactionTypeSell {
		return badRequest(c, "type must be BUY or SELL")
	}
	ts, err := parseDate(req.Timestamp)
	if err != nil {
		return badRequest(c, "invalid timestamp")
	}
	t := model.Transaction{
		StrategyID:  sid,
		Type:        req.Type,
		Shares:      req.Shares,
		Price:       req.Price,
		FeeCurrency: strings.TrimSpace(req.FeeCurrency),
		FeeAmount:   req.FeeAmount,
		Timestamp:   ts,
	}
	if err := h.db.Create(&t).Error; err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusCreated, t)
}

// --- helpers ---

func parseID(c echo.Context) (uuid.UUID, error) { return uuid.Parse(c.Param("id")) }

// parseDate accepts YYYY-MM-DD or RFC3339 (UTC assumed for the date-only form).
func parseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", s)
}

// parseRange turns a lookback token (e.g. "1y", "6m", "30d", "all"/"") into an
// absolute lower bound. Unrecognized or "all" ⇒ zero time (no filtering).
func parseRange(s string, now time.Time) time.Time {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || s == "all" {
		return time.Time{}
	}
	unit := s[len(s)-1]
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n <= 0 {
		return time.Time{}
	}
	switch unit {
	case 'd':
		return now.AddDate(0, 0, -n)
	case 'm':
		return now.AddDate(0, -n, 0)
	case 'y':
		return now.AddDate(-n, 0, 0)
	default:
		return time.Time{}
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func badRequest(c echo.Context, msg string) error {
	return c.JSON(http.StatusBadRequest, map[string]string{"error": msg})
}

func serverErr(c echo.Context, err error) error {
	return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

func notFoundOrErr(c echo.Context, err error) error {
	if err == gorm.ErrRecordNotFound {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}
	return serverErr(c, err)
}
