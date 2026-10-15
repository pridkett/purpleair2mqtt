package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TimescaleDB settings. The connection comes from the standard PG*
// environment variables (PGHOST, PGUSER, PGPASSWORD, PGDATABASE, ...), so
// the config file holds no credentials.
type tomlConfigTimescale struct {
	// Table gets one row per channel (A and B) per reading; SystemTable one
	// row per reading with the BME280 and wifi values.
	Table       string
	SystemTable string
	// MaxQueue caps the readings held per table while the database is
	// unreachable.
	MaxQueue int
}

var errSchema = errors.New("schema mismatch")

// bme280Failed is the temperature (°F) a PurpleAir reports once its BME280
// has failed (it reads about -227); its humidity, dewpoint and pressure are
// then meaningless too.
const bme280Failed = -200

// pgWriter inserts rows into one wide table. Rows that can't be written are
// queued, oldest first, and retried with the next ones.
type pgWriter struct {
	pool     *pgxpool.Pool
	table    string
	columns  []string
	maxQueue int
	queue    [][]any
	checked  bool
}

func newPGPool(ctx context.Context) (*pgxpool.Pool, error) {
	pgcfg, err := pgxpool.ParseConfig("")
	if err != nil {
		return nil, fmt.Errorf("timescale: %w", err)
	}
	pgcfg.ConnConfig.RuntimeParams["application_name"] = "purpleair2mqtt"
	pgcfg.MaxConns = 2
	return pgxpool.NewWithConfig(ctx, pgcfg)
}

func newPGWriter(pool *pgxpool.Pool, table string, rowType reflect.Type, maxQueue int) *pgWriter {
	if maxQueue <= 0 {
		maxQueue = 1000 // over a day at the default 120 s poll
	}
	return &pgWriter{
		pool:     pool,
		table:    table,
		columns:  append([]string{"time"}, pgColumns(rowType)...),
		maxQueue: maxQueue,
	}
}

// pgColumns lists the struct's pg-tagged columns in field order.
func pgColumns(t reflect.Type) []string {
	var cols []string
	for i := 0; i < t.NumField(); i++ {
		if name := t.Field(i).Tag.Get("pg"); name != "" && name != "-" {
			cols = append(cols, name)
		}
	}
	return cols
}

// pgRow is a value's pg-tagged fields in pgColumns order, after the time.
func pgRow(t time.Time, v any) []any {
	rv := reflect.Indirect(reflect.ValueOf(v))
	row := []any{t}
	for i := 0; i < rv.NumField(); i++ {
		if name := rv.Type().Field(i).Tag.Get("pg"); name != "" && name != "-" {
			row = append(row, rv.Field(i).Interface())
		}
	}
	return row
}

// systemRow is pgRow for the status, with the BME280 readings NULL once the
// sensor has failed.
func systemRow(t time.Time, status *purpleAirStatus) []any {
	row := pgRow(t, status)
	if status.Temperature <= bme280Failed {
		cols := append([]string{"time"}, pgColumns(reflect.TypeOf(*status))...)
		for i, c := range cols {
			switch c {
			case "temperature_f", "humidity", "dewpoint_f", "pressure":
				row[i] = nil
			}
		}
	}
	return row
}

// check verifies the table has every column, so a schema mismatch fails
// loudly instead of losing readings.
func (w *pgWriter) check(ctx context.Context) error {
	rows, err := w.pool.Query(ctx, `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1`, w.table)
	if err != nil {
		return err
	}
	have, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	if len(have) == 0 {
		return fmt.Errorf("%w: table %q doesn't exist", errSchema, w.table)
	}
	got := map[string]bool{}
	for _, c := range have {
		got[c] = true
	}
	var missing []string
	for _, c := range w.columns {
		if !got[c] {
			missing = append(missing, c)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: table %q is missing columns %s", errSchema, w.table, strings.Join(missing, ", "))
	}
	return nil
}

// write queues rows and flushes the queue. It returns the number of rows
// written; on error the rows stay queued for the next call.
func (w *pgWriter) write(ctx context.Context, rows ...[]any) (int, error) {
	w.queue = append(w.queue, rows...)
	if over := len(w.queue) - w.maxQueue; over > 0 {
		logger.Warnf("TimescaleDB queue for %s full, dropping the %d oldest rows", w.table, over)
		w.queue = w.queue[over:]
	}
	if !w.checked {
		if err := w.check(ctx); err != nil {
			return 0, err
		}
		w.checked = true
	}
	n, err := w.pool.CopyFrom(ctx, pgx.Identifier{w.table}, w.columns, pgx.CopyFromRows(w.queue))
	if err != nil {
		return 0, err
	}
	w.queue = w.queue[:0]
	return int(n), nil
}

// timescale writes each reading to both tables.
type timescale struct {
	monitors, system *pgWriter
}

func newTimescale(ctx context.Context, cfg tomlConfigTimescale) (*timescale, error) {
	if cfg.Table == "" || cfg.SystemTable == "" {
		return nil, fmt.Errorf("timescale: table and system_table are required")
	}
	pool, err := newPGPool(ctx)
	if err != nil {
		return nil, err
	}
	return &timescale{
		monitors: newPGWriter(pool, cfg.Table, reflect.TypeOf(purpleAirMonitor{}), cfg.MaxQueue),
		system:   newPGWriter(pool, cfg.SystemTable, reflect.TypeOf(purpleAirStatus{}), cfg.MaxQueue),
	}, nil
}

func (ts *timescale) write(ctx context.Context, t time.Time, status *purpleAirStatus) error {
	_, errM := ts.monitors.write(ctx, pgRow(t, &status.A), pgRow(t, &status.B))
	_, errS := ts.system.write(ctx, systemRow(t, status))
	return errors.Join(errM, errS)
}
