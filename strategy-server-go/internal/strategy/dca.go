package strategy

import "time"

// DCA contributes a fixed increment every interval, independent of the
// portfolio's actual value. It needs none of VA's target math — its only
// extra curve is the flat increment itself, always answerable regardless
// of whether a price exists at a given point.
type DCA struct {
	pf portfolio
}

// GetHistory returns invested/actual (universal) plus DCA's flat
// recommended-contribution curve, in the requested display currency.
func (d DCA) GetHistory(display string, rangeFrom, now time.Time, maxPoints int) (*History, error) {
	dates := d.pf.dates(rangeFrom, now, maxPoints)
	h, err := d.pf.history(dates)
	if err != nil {
		return nil, err
	}

	for i := range h.Points {
		h.Points[i].Values["recommended_contribution"] = d.pf.increment
	}
	h.Series = append(h.Series, SeriesMeta{Key: "recommended_contribution", Label: "Recommended Contribution"})

	if err := d.pf.toDisplay(h, display); err != nil {
		return nil, err
	}
	return h, nil
}

// GetSummary returns total-invested/current-value/gain-loss (universal)
// plus DCA's flat next-recommended-contribution, always present regardless
// of PriceStale.
func (d DCA) GetSummary(display string, now time.Time) (*Summary, error) {
	sum, err := d.pf.summary(now)
	if err != nil {
		return nil, err
	}

	sum.Values["next_recommended_contribution"] = d.pf.increment

	if err := d.pf.toDisplaySummary(sum, display); err != nil {
		return nil, err
	}
	return sum, nil
}

func (d DCA) DisplayCurrencies() ([]string, string, error) {
	return d.pf.displayCurrencies()
}
