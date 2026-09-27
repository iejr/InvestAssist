// Command krakencsv is a one-off importer for Kraken's downloadable historical
// OHLCVT dump (https://support.kraken.com/hc/en-us/articles/360047124832),
// used to backfill history older than what Kraken's live OHLC REST endpoint
// can serve (it has no date-range param and caps at 720 rows from "now").
//
// It reads every "<PAIR>_<MINUTES>.csv" file in a directory (e.g.
// "USDCUSD_15.csv"), splits <PAIR> into base/quote the same way the rest of
// market-feed does, and writes rows into price_candles. It is not limited to
// stablecoins: any pair whose quote suffix is recognized by normalize.Symbol
// is imported.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"market-feed/internal/config"
	"market-feed/internal/db"
	"market-feed/internal/model"
	"market-feed/internal/normalize"
	"market-feed/internal/repository"
)

// filenamePattern matches Kraken's OHLCVT dump filenames: a concatenated pair
// (Kraken's own ticker, e.g. "USDCUSD") followed by the candle interval in
// minutes.
var filenamePattern = regexp.MustCompile(`^([A-Za-z0-9]+)_(\d+)\.csv$`)

// minuteIntervals maps Kraken's minute-granularity file suffix to our stored
// interval labels. Kraken's dump also ships 15/30/240/720-minute files; those
// have no matching model.Interval (the price_candles CHECK constraint only
// allows 1s,5s,1m,5m,1h,1d), so files at those granularities are skipped.
var minuteIntervals = map[int]model.Interval{
	1:    model.Interval1m,
	5:    model.Interval5m,
	60:   model.Interval1h,
	1440: model.Interval1d,
}

// flushEvery bounds how many candles are buffered before writing, so a
// multi-year 1-minute file doesn't have to be held in memory all at once.
const flushEvery = 5000

// candleWriter is the subset of repository.CandleRepo this importer needs.
type candleWriter interface {
	InsertBatch(candles []model.PriceCandle) error
	UpsertBatch(candles []model.PriceCandle) error
}

func main() {
	dir := flag.String("dir", "", "directory containing extracted Kraken OHLCVT csv files (required)")
	source := flag.String("source", string(model.SourceKraken), "source label to tag imported candles with")
	override := flag.Bool("override", true, "overwrite existing candles for the same bucket (false = fill gaps only)")
	flag.Parse()

	if *dir == "" {
		log.Fatal("-dir is required")
	}

	cfg := config.Load()
	conns, err := db.Open(cfg.DatabaseURL, cfg.HistoryDatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	candleRepo := repository.NewCandleRepo(conns.History)

	entries, err := os.ReadDir(*dir)
	if err != nil {
		log.Fatalf("read dir: %v", err)
	}

	var filesOK, filesSkipped, rowsWritten int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n, err := importFile(*dir, e, model.Source(*source), *override, candleRepo)
		if err != nil {
			log.Printf("skip %s: %v", e.Name(), err)
			filesSkipped++
			continue
		}
		filesOK++
		rowsWritten += n
	}
	log.Printf("kraken csv import: %d file(s) imported, %d skipped, %d candle rows written", filesOK, filesSkipped, rowsWritten)
}

// importFile parses one CSV file and writes its rows, flushing in chunks of
// flushEvery. It returns the number of candles written.
func importFile(dir string, e fs.DirEntry, source model.Source, override bool, store candleWriter) (int, error) {
	m := filenamePattern.FindStringSubmatch(e.Name())
	if m == nil {
		return 0, fmt.Errorf("filename doesn't match PAIR_MINUTES.csv")
	}
	pairRaw, minutesRaw := m[1], m[2]

	minutes, err := strconv.Atoi(minutesRaw)
	if err != nil {
		return 0, fmt.Errorf("bad interval %q: %w", minutesRaw, err)
	}
	iv, ok := minuteIntervals[minutes]
	if !ok {
		return 0, fmt.Errorf("%d-minute interval has no matching stored interval", minutes)
	}

	base, quote, ok := normalize.Symbol(pairRaw)
	if !ok {
		return 0, fmt.Errorf("unrecognized pair %q (no known quote suffix)", pairRaw)
	}

	f, err := os.Open(filepath.Join(dir, e.Name()))
	if err != nil {
		return 0, err
	}
	defer f.Close()

	write := func(batch []model.PriceCandle) error {
		if override {
			return store.UpsertBatch(batch)
		}
		return store.InsertBatch(batch)
	}

	r := csv.NewReader(f)
	batch := make([]model.PriceCandle, 0, flushEvery)
	total := 0
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return total, fmt.Errorf("csv read: %w", err)
		}
		c, err := parseRow(row, base, quote, iv, source)
		if err != nil {
			return total, err
		}
		batch = append(batch, c)
		if len(batch) >= flushEvery {
			if err := write(batch); err != nil {
				return total, fmt.Errorf("write: %w", err)
			}
			total += len(batch)
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		if err := write(batch); err != nil {
			return total, fmt.Errorf("write: %w", err)
		}
		total += len(batch)
	}

	log.Printf("%s: %s/%s %s: wrote %d candles", e.Name(), base, quote, iv, total)
	return total, nil
}

// parseRow converts one Kraken OHLCVT row: timestamp(sec),open,high,low,close,
// volume,trades. Trade count is dropped; price_candles has no column for it.
func parseRow(row []string, base, quote string, iv model.Interval, source model.Source) (model.PriceCandle, error) {
	if len(row) < 6 {
		return model.PriceCandle{}, fmt.Errorf("malformed row (len %d): %v", len(row), row)
	}
	sec, err := strconv.ParseInt(row[0], 10, 64)
	if err != nil {
		return model.PriceCandle{}, fmt.Errorf("bad timestamp %q: %w", row[0], err)
	}
	open, err1 := strconv.ParseFloat(row[1], 64)
	high, err2 := strconv.ParseFloat(row[2], 64)
	low, err3 := strconv.ParseFloat(row[3], 64)
	cls, err4 := strconv.ParseFloat(row[4], 64)
	vol, err5 := strconv.ParseFloat(row[5], 64)
	if err := firstErr(err1, err2, err3, err4, err5); err != nil {
		return model.PriceCandle{}, fmt.Errorf("bad ohlcv row %v: %w", row, err)
	}
	return model.PriceCandle{
		Base:     base,
		Quote:    quote,
		Interval: iv,
		OpenTime: time.Unix(sec, 0).UTC(),
		Open:     open,
		High:     high,
		Low:      low,
		Close:    cls,
		Volume:   vol,
		Source:   source,
	}, nil
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}
