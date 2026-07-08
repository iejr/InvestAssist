package sampler

import (
	"context"
	"log"
	"time"

	"market-feed/internal/model"
	"market-feed/internal/provider"
	"market-feed/internal/repository"
)

// Sampler aggregates incoming PriceEvents into fixed-interval OHLC candles and
// appends them to storage. Incoming frequency and storage frequency are
// independent: many trades per second collapse into one candle per interval.
type Sampler struct {
	interval time.Duration
	label    model.Interval
	repo     *repository.CandleRepo
}

// bucket accumulates the OHLC for one (edge, open_time) window.
type bucket struct {
	base, quote string
	source      model.Source
	openTime    time.Time
	open        float64
	high        float64
	low         float64
	close       float64
	volume      float64
}

// New creates a Sampler that flushes one candle per interval. The interval is
// mapped to a stored label (e.g. 1m); unknown intervals still store using the
// closest canonical label.
func New(interval time.Duration, repo *repository.CandleRepo) *Sampler {
	return &Sampler{
		interval: interval,
		label:    labelFor(interval),
		repo:     repo,
	}
}

// Run consumes events until the channel closes or ctx is cancelled. On each
// interval boundary it flushes completed buckets. Any open buckets are flushed
// on shutdown so no observed data is silently dropped.
func (s *Sampler) Run(ctx context.Context, in <-chan provider.PriceEvent) {
	buckets := make(map[string]*bucket)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	flush := func(now time.Time) {
		for key, b := range buckets {
			// Flush buckets whose window has closed.
			if !b.openTime.Add(s.interval).After(now) {
				s.store(b)
				delete(buckets, key)
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			s.flushAll(buckets)
			return

		case ev, ok := <-in:
			if !ok {
				s.flushAll(buckets)
				return
			}
			s.add(buckets, ev)

		case t := <-ticker.C:
			flush(t.UTC())
		}
	}
}

// add folds an event into its bucket, creating the bucket if needed.
func (s *Sampler) add(buckets map[string]*bucket, ev provider.PriceEvent) {
	openTime := ev.Timestamp.UTC().Truncate(s.interval)
	key := ev.Base + "/" + ev.Quote + "@" + openTime.Format(time.RFC3339)

	b, ok := buckets[key]
	if !ok {
		b = &bucket{
			base:     ev.Base,
			quote:    ev.Quote,
			source:   ev.Source,
			openTime: openTime,
			open:     ev.Price,
			high:     ev.Price,
			low:      ev.Price,
		}
		buckets[key] = b
	}
	if ev.Price > b.high {
		b.high = ev.Price
	}
	if ev.Price < b.low {
		b.low = ev.Price
	}
	b.close = ev.Price
	b.volume += ev.Volume
}

func (s *Sampler) flushAll(buckets map[string]*bucket) {
	for key, b := range buckets {
		s.store(b)
		delete(buckets, key)
	}
}

func (s *Sampler) store(b *bucket) {
	err := s.repo.Insert(model.PriceCandle{
		Base:     b.base,
		Quote:    b.quote,
		Interval: s.label,
		OpenTime: b.openTime,
		Open:     b.open,
		High:     b.high,
		Low:      b.low,
		Close:    b.close,
		Volume:   b.volume,
		Source:   b.source,
	})
	if err != nil {
		log.Printf("sampler: store %s/%s @ %s: %v", b.base, b.quote, b.openTime, err)
	}
}

// labelFor maps a sampling duration to the stored interval label.
func labelFor(d time.Duration) model.Interval {
	switch d {
	case time.Second:
		return model.Interval1s
	case 5 * time.Second:
		return model.Interval5s
	case time.Minute:
		return model.Interval1m
	case 5 * time.Minute:
		return model.Interval5m
	case time.Hour:
		return model.Interval1h
	case 24 * time.Hour:
		return model.Interval1d
	default:
		return model.Interval1m
	}
}
