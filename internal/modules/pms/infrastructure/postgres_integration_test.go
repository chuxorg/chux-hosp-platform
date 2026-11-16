//go:build integration

package infrastructure

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/api"
	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/application"
	"github.com/chuxorg/chux-hosp-platform/internal/modules/pms/domain"
	"github.com/chuxorg/chux-hosp-platform/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func openIsolatedPMSDatabase(t *testing.T) (string, *pgxpool.Pool) {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	cfg, err := pgx.ParseConfig(raw)
	if err != nil || raw == "" || cfg.Database == "_test" || !strings.HasSuffix(cfg.Database, "_test") {
		t.Fatal("TEST_DATABASE_URL must point to a dedicated *_test provisioning DB")
	}
	if appURL := os.Getenv("DATABASE_URL"); appURL != "" {
		app, err := pgx.ParseConfig(appURL)
		if err != nil {
			t.Fatal("invalid application DATABASE_URL")
		}
		if app.Host == cfg.Host && app.Port == cfg.Port && app.Database == cfg.Database {
			t.Fatal("TEST_DATABASE_URL must be separate from the application database")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	admin, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal("cannot connect to dedicated test provisioning database")
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	suffix := make([]byte, 12)
	if _, err = rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	name := "pms_it_" + hex.EncodeToString(suffix) + "_test"
	ident := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+ident); err != nil {
		t.Fatal("test role needs CREATEDB privilege")
	}
	// Cleanup is registered before pool.Close(), so pool closes first (LIFO).
	t.Cleanup(func() {
		c, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if _, err := admin.Exec(c, "DROP DATABASE "+ident+" WITH (FORCE)"); err != nil {
			t.Error("could not drop isolated test database")
		}
	})
	// ConnConfig.ConnString() preserves the original DSN even after Database
	// is modified. Explicitly target and then verify the unique child database
	// before any migration or DML can run.
	childURL, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	childURL.Path = "/" + name
	params := childURL.Query()
	params.Del("dbname")
	childURL.RawQuery = params.Encode()
	databaseURL := childURL.String()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var connectedName string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&connectedName); err != nil || connectedName != name {
		t.Fatal("test database URL did not target the isolated child")
	}
	return databaseURL, pool
}

func jsonRequest(h http.Handler, method, path, body string) (int, []byte) {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code, w.Body.Bytes()
}
func assertStatus(t *testing.T, got, want int, body []byte) {
	t.Helper()
	if got != want {
		t.Fatalf("HTTP %d, expected %d: %s", got, want, string(body))
	}
}
func parseResponse[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// TST-PMS-001-01: real PostgreSQL migration + full property/type/room CRUD,
// invalid references/duplicates, concurrency, and no partial writes.
func TestPMSConfigurationAgainstPostgres(t *testing.T) {
	databaseURL, pool := openIsolatedPMSDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	latestVersion, err := migrations.Run(ctx, databaseURL, "up")
	if err != nil || latestVersion < 2 {
		t.Fatalf("migrate up: version %d err %v", latestVersion, err)
	}
	h := api.Handler(application.NewService(NewStore(pool)))
	post := func(path, body string, expect int) []byte {
		t.Helper()
		got, b := jsonRequest(h, "POST", path, body)
		assertStatus(t, got, expect, b)
		return b
	}
	put := func(path, body string, expect int) []byte {
		t.Helper()
		got, b := jsonRequest(h, "PUT", path, body)
		assertStatus(t, got, expect, b)
		return b
	}
	get := func(path string, expect int) []byte {
		t.Helper()
		got, b := jsonRequest(h, "GET", path, "")
		assertStatus(t, got, expect, b)
		return b
	}
	p1 := parseResponse[domain.Property](t, post("/properties", `{"name":"  Pilot Hotel  ","time_zone":"Asia/Bangkok","active":true}`, 201))
	if p1.ID <= 0 || p1.Name != "Pilot Hotel" || p1.TimeZone != "Asia/Bangkok" || !p1.Active {
		t.Fatalf("property %+v", p1)
	}
	p2 := parseResponse[domain.Property](t, post("/properties", `{"name":"Second Hotel","time_zone":"UTC","active":true}`, 201))
	if p2.ID == p1.ID {
		t.Fatal("duplicate property IDs")
	}
	p1Path := "/properties/" + strconv.FormatInt(p1.ID, 10)
	gotProperty := parseResponse[domain.Property](t, get(p1Path, 200))
	if gotProperty != p1 {
		t.Fatal("read back mismatch")
	}
	updated := parseResponse[domain.Property](t, put(p1Path, `{"name":"Pilot Hotel 2","time_zone":"UTC","active":false}`, 200))
	if updated.Name != "Pilot Hotel 2" || updated.Active {
		t.Fatal("property update failed")
	}
	post("/properties", `{"name":"bad","time_zone":"Local","active":true}`, 400)
	post("/properties", `{"name":"bad","time_zone":"Invalid/Zone","active":true}`, 400)
	post("/properties", `{"name":"bad","time_zone":"UTC","active":true,"extra":1}`, 400)
	post("/properties", `{"name":"bad","time_zone":"UTC"}`, 400)
	post("/properties", `{"name":"bad","time_zone":"UTC","active":true} {"extra":1}`, 400)
	get("/properties/999999999", 404)
	get("/properties/nope", 400)
	typURL := p1Path + "/room-types"
	typ := parseResponse[domain.RoomType](t, post(typURL, `{"name":"  Standard ","max_occupancy":2,"active":true}`, 201))
	if typ.PropertyID != p1.ID || typ.Name != "Standard" || typ.MaxOccupancy != 2 {
		t.Fatalf("room type %+v", typ)
	}
	post(typURL, `{"name":"standard","max_occupancy":2,"active":true}`, 409)
	post(typURL, `{"name":"Invalid","max_occupancy":0,"active":true}`, 400)
	post(typURL, `{"name":"Invalid","max_occupancy":-1,"active":true}`, 400)
	post("/properties/999999999/room-types", `{"name":"Invalid","max_occupancy":2,"active":true}`, 422)
	typPath := typURL + "/" + strconv.FormatInt(typ.ID, 10)
	typ = parseResponse[domain.RoomType](t, put(typPath, `{"name":"Standard Plus","max_occupancy":3,"active":false}`, 200))
	if typ.MaxOccupancy != 3 || typ.Active {
		t.Fatal("room type not updated")
	}
	if parseResponse[domain.RoomType](t, get(typPath, 200)) != typ {
		t.Fatal("type read mismatch")
	}
	otherType := parseResponse[domain.RoomType](t, post("/properties/"+strconv.FormatInt(p2.ID, 10)+"/room-types", `{"name":"Standard","max_occupancy":2,"active":true}`, 201))
	roomsURL := p1Path + "/rooms"
	room := parseResponse[domain.Room](t, post(roomsURL, `{"room_type_id":`+strconv.FormatInt(typ.ID, 10)+`,"number":" A-101 ","active":true}`, 201))
	if room.Number != "A-101" || room.PropertyID != p1.ID || room.RoomTypeID != typ.ID {
		t.Fatalf("room %+v", room)
	}
	post(roomsURL, `{"room_type_id":`+strconv.FormatInt(typ.ID, 10)+`,"number":"a-101","active":true}`, 409)
	post(roomsURL, `{"room_type_id":`+strconv.FormatInt(otherType.ID, 10)+`,"number":"BAD","active":true}`, 422)
	post(roomsURL, `{"room_type_id":99999999,"number":"BAD","active":true}`, 422)
	post(roomsURL, `{"room_type_id":`+strconv.FormatInt(typ.ID, 10)+`,"number":" ","active":true}`, 400)
	// Verify invalid parent creates no partial record.
	var badCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM pms_rooms WHERE number='BAD'").Scan(&badCount); err != nil || badCount != 0 {
		t.Fatal("invalid parent changed data")
	}
	roomPath := roomsURL + "/" + strconv.FormatInt(room.ID, 10)
	room = parseResponse[domain.Room](t, put(roomPath, `{"room_type_id":`+strconv.FormatInt(typ.ID, 10)+`,"number":"A-102","active":false}`, 200))
	if room.Number != "A-102" || room.Active {
		t.Fatal("room update failed")
	}
	if parseResponse[domain.Room](t, get(roomPath, 200)) != room {
		t.Fatal("room read mismatch")
	}
	get("/properties/"+strconv.FormatInt(p2.ID, 10)+"/rooms/"+strconv.FormatInt(room.ID, 10), 404)
	// Atomic uniqueness is enforced by PostgreSQL, including simultaneous writers.
	var wg sync.WaitGroup
	statuses := make([]int, 2)
	for i := range statuses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			statuses[i], _ = jsonRequest(h, "POST", roomsURL, `{"room_type_id":`+strconv.FormatInt(typ.ID, 10)+`,"number":"RACE","active":true}`)
		}(i)
	}
	wg.Wait()
	if !((statuses[0] == 201 && statuses[1] == 409) || (statuses[0] == 409 && statuses[1] == 201)) {
		t.Fatalf("concurrent uniqueness statuses = %v", statuses)
	}
	// A rejected update must not modify an already-persisted room.
	another := parseResponse[domain.Room](t, post(roomsURL, `{"room_type_id":`+strconv.FormatInt(typ.ID, 10)+`,"number":"OTHER","active":true}`, 201))
	put(roomsURL+"/"+strconv.FormatInt(another.ID, 10), `{"room_type_id":`+strconv.FormatInt(typ.ID, 10)+`,"number":"a-102","active":false}`, 409)
	if actual := parseResponse[domain.Room](t, get(roomsURL+"/"+strconv.FormatInt(another.ID, 10), 200)); actual.Number != "OTHER" || !actual.Active {
		t.Fatalf("failed update modified room: %+v", actual)
	}
	// The database is disposable. Remove dependent PMS migrations before the
	// original configuration migration; retain its rollback assertions.
	for current := latestVersion; current > 1; current-- {
		if version, err := migrations.Run(ctx, databaseURL, "down"); err != nil || version != current-1 {
			t.Fatalf("PMS rollback version %d: %v", version, err)
		}
	}
	var missing bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('pms_rooms') IS NULL").Scan(&missing); err != nil || !missing {
		t.Fatal("PMS rollback left room tables")
	}
	if version, err := migrations.Run(ctx, databaseURL, "up"); err != nil || version != latestVersion {
		t.Fatalf("PMS reapply version %d: %v", version, err)
	}
}
