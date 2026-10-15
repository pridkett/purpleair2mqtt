package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/withmandala/go-log"
)

func TestMain(m *testing.M) {
	logger = log.New(os.Stderr)
	os.Exit(m.Run())
}

// A PA-SD-II with a failed BME280, captured 2026-10-15 (location removed).
func loadFixture(t *testing.T) *purpleAirStatus {
	t.Helper()
	b, err := os.ReadFile("testdata/pa-sd-ii.json")
	if err != nil {
		t.Fatal(err)
	}
	s := new(purpleAirStatus)
	if err := json.Unmarshal(b, s); err != nil {
		t.Fatal(err)
	}
	return normalizePaStatus(s)
}

// The columns must match the tables in ansible-config's environment.sql.j2.
func TestPGColumns(t *testing.T) {
	for typ, want := range map[reflect.Type]string{
		reflect.TypeOf(purpleAirMonitor{}): "sensor_id channel pm2_5_aqi pm1_0_cf_1 p_0_3_um pm2_5_cf_1 p_0_5_um " +
			"pm10_0_cf_1 p_1_0_um pm1_0_atm p_2_5_um pm2_5_atm p_5_0_um pm10_0_atm p_10_0_um",
		reflect.TypeOf(purpleAirStatus{}): "sensor_id rssi temperature_f humidity dewpoint_f pressure",
	} {
		if got := strings.Join(pgColumns(typ), " "); got != want {
			t.Errorf("%s columns\n got %s\nwant %s", typ, got, want)
		}
	}
}

func rowMap(cols []string, row []any) map[string]any {
	m := map[string]any{}
	for i, c := range append([]string{"time"}, cols...) {
		m[c] = row[i]
	}
	return m
}

func TestRows(t *testing.T) {
	s := loadFixture(t)
	ts := time.Date(2026, 10, 15, 16, 28, 11, 0, time.UTC)
	monCols := pgColumns(reflect.TypeOf(purpleAirMonitor{}))

	a, b := rowMap(monCols, pgRow(ts, &s.A)), rowMap(monCols, pgRow(ts, &s.B))
	if a["channel"] != "A" || b["channel"] != "B" || a["sensor_id"] != "40:f5:20:1d:b4:18" || b["sensor_id"] != "40:f5:20:1d:b4:18" {
		t.Errorf("channel/sensor_id: A %v %v, B %v %v", a["channel"], a["sensor_id"], b["channel"], b["sensor_id"])
	}
	if a["pm2_5_atm"] != 0.0 || b["pm2_5_atm"] != 0.32 || b["p_0_3_um"] != s.P03umB || a["time"] != ts {
		t.Errorf("A %v, B %v", a, b)
	}

	// The BME280 has failed: its readings are NULL, rssi is kept
	sys := rowMap(pgColumns(reflect.TypeOf(purpleAirStatus{})), systemRow(ts, s))
	for _, c := range []string{"temperature_f", "humidity", "dewpoint_f", "pressure"} {
		if sys[c] != nil {
			t.Errorf("%s = %v, want NULL with a failed BME280", c, sys[c])
		}
	}
	if sys["rssi"] != s.RSSI || sys["sensor_id"] != s.SensorId {
		t.Errorf("system row %v", sys)
	}

	// A working BME280 keeps its readings
	s.Temperature, s.Dewpoint, s.Pressure = 61, 48, 1012.3
	sys = rowMap(pgColumns(reflect.TypeOf(purpleAirStatus{})), systemRow(ts, s))
	if sys["temperature_f"] != 61 || sys["dewpoint_f"] != 48 || sys["pressure"] != 1012.3 || sys["humidity"] != s.Humidity {
		t.Errorf("system row %v", sys)
	}

	// A dropped-out read is all zeros; glitches null just the bad reading
	for _, c := range []struct {
		temp, hum, dew int
		pressure       float64
		null           string
	}{
		{0, 0, 0, 0, "temperature_f humidity dewpoint_f pressure"},
		{373, 100, 50, 1012, "temperature_f humidity dewpoint_f"},
		{61, 40, 48, -228.79, "pressure"},
		{0, 40, -10, 1012, ""}, // a real 0 °F winter reading
	} {
		s.Temperature, s.Humidity, s.Dewpoint, s.Pressure = c.temp, c.hum, c.dew, c.pressure
		sys = rowMap(pgColumns(reflect.TypeOf(purpleAirStatus{})), systemRow(ts, s))
		var null []string
		for _, col := range []string{"temperature_f", "humidity", "dewpoint_f", "pressure"} {
			if sys[col] == nil {
				null = append(null, col)
			}
		}
		if got := strings.Join(null, " "); got != c.null {
			t.Errorf("%+v: NULL %q, want %q", c, got, c.null)
		}
	}
}

// Without a database, rows queue up to MaxQueue, dropping the oldest.
func TestQueueWhileDown(t *testing.T) {
	t.Setenv("PGHOST", "127.0.0.1")
	t.Setenv("PGPORT", "1")
	t.Setenv("PGCONNECT_TIMEOUT", "1")
	ts, err := newTimescale(context.Background(), tomlConfigTimescale{Table: "purpleair", SystemTable: "purpleair_system", MaxQueue: 3})
	if err != nil {
		t.Fatal(err)
	}
	s := loadFixture(t)
	for i := range 3 {
		if err := ts.write(context.Background(), time.Unix(int64(i), 0), s); err == nil {
			t.Fatal("write succeeded with no database")
		}
	}
	// two channel rows per reading, capped at 3
	if len(ts.monitors.queue) != 3 || ts.monitors.queue[0][0] != time.Unix(1, 0) || len(ts.system.queue) != 3 {
		t.Errorf("queues: %d monitor rows (first %v), %d system rows", len(ts.monitors.queue), ts.monitors.queue[0][0], len(ts.system.queue))
	}
}

// TestTimescale runs against a real database when PGHOST is set, e.g. a
// local timescale/timescaledb container.
func TestTimescale(t *testing.T) {
	if os.Getenv("PGHOST") == "" {
		t.Skip("PGHOST not set")
	}
	ctx := context.Background()
	ts, err := newTimescale(ctx, tomlConfigTimescale{Table: "purpleair_test", SystemTable: "purpleair_system_test"})
	if err != nil {
		t.Fatal(err)
	}
	pool := ts.monitors.pool
	defer pool.Exec(ctx, `DROP TABLE IF EXISTS purpleair_test, purpleair_system_test`)

	s := loadFixture(t)
	if err := ts.write(ctx, time.Now(), s); !errors.Is(err, errSchema) {
		t.Fatalf("missing tables: err = %v", err)
	}
	ts.monitors.queue, ts.system.queue = nil, nil
	if _, err := pool.Exec(ctx, testSchema); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 15, 16, 28, 11, 0, time.UTC)
	if err := ts.write(ctx, now, s); err != nil {
		t.Fatal(err)
	}
	var n int
	var pm25b float64
	if err := pool.QueryRow(ctx, `SELECT count(*), max(pm2_5_atm) FILTER (WHERE channel = 'B') FROM purpleair_test`).Scan(&n, &pm25b); err != nil || n != 2 || pm25b != 0.32 {
		t.Errorf("purpleair_test: %d rows, B pm2_5_atm %v, %v", n, pm25b, err)
	}
	var temp *int
	var rssi int
	if err := pool.QueryRow(ctx, `SELECT temperature_f, rssi FROM purpleair_system_test`).Scan(&temp, &rssi); err != nil || temp != nil || rssi != s.RSSI {
		t.Errorf("purpleair_system_test: temp %v rssi %d, %v", temp, rssi, err)
	}
}

const testSchema = `
CREATE TABLE purpleair_test (time timestamptz NOT NULL, sensor_id text NOT NULL, channel text NOT NULL,
  pm2_5_aqi integer, pm1_0_cf_1 double precision, p_0_3_um double precision, pm2_5_cf_1 double precision,
  p_0_5_um double precision, pm10_0_cf_1 double precision, p_1_0_um double precision, pm1_0_atm double precision,
  p_2_5_um double precision, pm2_5_atm double precision, p_5_0_um double precision, pm10_0_atm double precision,
  p_10_0_um double precision);
CREATE TABLE purpleair_system_test (time timestamptz NOT NULL, sensor_id text NOT NULL, rssi integer,
  temperature_f integer, humidity integer, dewpoint_f integer, pressure double precision);`

// InfluxDB rejects a field whose type changes: the measurements' fields must
// keep the types they had (floats stay floats now that they're float64).
func TestInfluxFieldTypes(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	config.Influx = tomlConfigInflux{Hostname: u.Hostname(), Port: port, Database: "purpleair"}

	s := loadFixture(t)
	write_influx(s, &s.A, &s.B)
	for _, want := range []string{"pm2.5_atm=0.32,", "pm2.5_aqi=1i", "temperature=-227i", "pressure=", "rssi=-"} {
		if !strings.Contains(body, want) {
			t.Errorf("line protocol is missing %s:\n%s", want, body)
		}
	}
}
