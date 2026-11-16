//go:build regression

package rental

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var testDB *pgxpool.Pool
var testEquipment *PostgresEquipmentRepository
var testRentals *PostgresRentalRepository
var testService *RentalService
var testContext = context.Background()

func TestMain(m *testing.M) { os.Exit(runRegression(m)) }
func runRegression(m *testing.M) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{
		Image: "postgres:17.6-alpine", ExposedPorts: []string{"5432/tcp"}, Env: map[string]string{"POSTGRES_USER": "test", "POSTGRES_PASSWORD": "test", "POSTGRES_DB": "test"}, WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(time.Minute),
	}, Started: true})
	if container != nil {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := container.Terminate(ctx); err != nil {
				fmt.Fprintln(os.Stderr, "Не вдалося видалити контейнер:", err)
			}
		}()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Помилка запуску PostgreSQL:", err)
		return 1
	}
	host, err := container.Host(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	testDB, err = pgxpool.New(ctx, fmt.Sprintf("postgres://test:test@%s:%s/test?sslmode=disable", host, port.Port()))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer testDB.Close()
	if err = testDB.Ping(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	files, err := filepath.Glob("migrations/V*__*.sql")
	if err != nil || len(files) == 0 {
		fmt.Fprintln(os.Stderr, "Міграції не знайдено")
		return 1
	}
	sort.Strings(files)
	for _, file := range files {
		sql, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if _, err = testDB.Exec(ctx, string(sql)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	testEquipment = &PostgresEquipmentRepository{DB: testDB}
	testRentals = &PostgresRentalRepository{DB: testDB}
	testService, err = NewRentalService(testEquipment, testRentals, PolicyFunc(func(context.Context, Equipment, int, int) (bool, error) { return true, nil }))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return m.Run()
}
func uniqueCode() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func must[T any](t *testing.T, value T, err error) T {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func createEquipmentFor(t *testing.T, dailyRate int64, stock int) Equipment {
	t.Helper()

	e, err := testEquipment.Create(testContext, Equipment{Code: uniqueCode(), DailyRate: dailyRate, Stock: stock})
	saved := must(t, e, err)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if _, err := testDB.Exec(ctx, `DELETE FROM rentals WHERE id IN (SELECT rental_id 
                                 FROM rental_items WHERE equipment_id = $1)`, saved.ID); err != nil {
			t.Errorf("Не вдалося очистити оренди обладнання %d: %v", saved.ID, err)
		}

		if err := testEquipment.Delete(ctx, saved.ID); err != nil {
			t.Errorf("Не вдалося очистити обладнання %d: %v", saved.ID, err)
		}
	})

	return saved
}

func equipmentFor(t *testing.T) Equipment {
	t.Helper()
	return createEquipmentFor(t, 10000, 5)
}
func rentalRequest(e Equipment) RentalRequest {
	return RentalRequest{Reference: uniqueCode(), EquipmentID: e.ID, Quantity: 1, Days: 2}
}
func rentFor(t *testing.T, e Equipment) RentalOrder {
	t.Helper()
	r, err := testService.Rent(testContext, rentalRequest(e))
	return must(t, r, err)
}
func stored(t *testing.T, id int64) RentalOrder {
	t.Helper()
	r, err := testRentals.FindByID(testContext, id)
	return must(t, r, err)
}
func activeFor(t *testing.T, id int64) []RentalOrder {
	t.Helper()
	r, err := testRentals.FindRentals(testContext, id, Active)
	return must(t, r, err)
}
func TestCreateRental(t *testing.T) {
	saved := rentFor(t, equipmentFor(t))

	if saved.Status != Active || stored(t, saved.ID) != saved {
		t.Fatal("Збережена оренда не відповідає результату")
	}
}

func TestReturnRental(t *testing.T) {
	saved := rentFor(t, equipmentFor(t))

	r, err := testService.ReturnRental(testContext, saved.ID)
	returned := must(t, r, err)

	if returned.Status != Returned ||
		returned.LateFee != 0 ||
		stored(t, saved.ID) != returned {
		t.Fatal("Некоректний результат повернення")
	}
}
func TestEquipmentStock(t *testing.T) {
	saved := createEquipmentFor(t, 10000, 5)

	e, err := testEquipment.FindByID(testContext, saved.ID)
	if must(t, e, err).Stock != 5 {
		t.Fatal("Некоректний залишок")
	}
}

func TestEquipmentRate(t *testing.T) {
	saved := createEquipmentFor(t, 15000, 3)

	e, err := testEquipment.FindByID(testContext, saved.ID)
	if must(t, e, err).DailyRate != 15000 {
		t.Fatal("Некоректна добова ціна")
	}
}
func TestRentalAfterPolicyCheck(t *testing.T) {
	e := equipmentFor(t)
	request := rentalRequest(e)

	delay := 200
	if value := os.Getenv("REGRESSION_POLICY_DELAY_MS"); value != "" {
		var err error
		delay, err = strconv.Atoi(value)
		if err != nil || delay < 0 || delay > 5000 {
			t.Fatal("REGRESSION_POLICY_DELAY_MS має бути від 0 до 5000")
		}
	}

	ctx, cancel := context.WithTimeout(testContext, 10*time.Second)
	defer cancel()

	delayed, err := NewRentalService(
		testEquipment,
		testRentals,
		PolicyFunc(func(ctx context.Context, _ Equipment, _, _ int) (bool, error) {
			timer := time.NewTimer(time.Duration(delay) * time.Millisecond)
			defer timer.Stop()

			select {
			case <-timer.C:
				return true, nil
			case <-ctx.Done():
				return false, ctx.Err()
			}
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	operation := make(chan error, 1)
	finished := make(chan struct{})

	go func() {
		defer close(finished)
		_, err := delayed.Rent(ctx, request)
		operation <- err
	}()

	defer func() {
		cancel()
		<-finished
	}()

	select {
	case err := <-operation:
		if err != nil {
			t.Fatalf("Помилка фонової операції: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("Не дочекалися завершення оренди: %v", ctx.Err())
	}

	rows := activeFor(t, e.ID)
	if len(rows) != 1 {
		t.Fatalf("Очікується 1 оренда, отримано %d", len(rows))
	}

	if rows[0].Reference != request.Reference || rows[0].Total != 20000 {
		t.Fatal("Некоректні дані оренди")
	}
}
func TestRentalSelection(t *testing.T) {
	e := equipmentFor(t)
	prefix := uniqueCode()[:8]
	refs := []string{prefix + "-A", prefix + "-B"}
	order := append([]string{}, refs...)
	if os.Getenv("REGRESSION_REVERSE_FIXTURE") == "true" {
		order[0], order[1] = order[1], order[0]
	}
	for _, ref := range order {
		req := rentalRequest(e)
		req.Reference = ref
		if _, err := testService.Rent(testContext, req); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := testDB.Query(testContext, `
	SELECT r.reference
	FROM rentals r
	JOIN rental_items i ON i.rental_id = r.id
	WHERE i.equipment_id = $1
	ORDER BY r.reference
`, e.ID)

	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	actual := []string{}
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			t.Fatal(err)
		}
		actual = append(actual, ref)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(actual) != 2 || actual[0] != refs[0] || actual[1] != refs[1] {
		t.Fatalf("Очікується %v, отримано %v", refs, actual)
	}
}
func TestRentalAmount(t *testing.T) {
	e := equipmentFor(t)
	saved := rentFor(t, e)
	r := stored(t, saved.ID)
	if r.Total != 20000 || r.Item.EquipmentID != e.ID || r.Item.Quantity != 1 || r.Item.DailyRate != 10000 {
		t.Fatal("Некоректна сума або позиція")
	}
}
func TestStockRejection(t *testing.T) {
	e := equipmentFor(t)
	req := rentalRequest(e)
	req.Quantity = 6
	if _, err := testService.Rent(testContext, req); !errors.Is(err, ErrConflict) {
		t.Fatalf("Очікується відмова, отримано %v", err)
	}
	if len(activeFor(t, e.ID)) != 0 {
		t.Fatal("Створено зайву оренду")
	}
}
func TestPolicyRejection(t *testing.T) {
	e := equipmentFor(t)
	denied, err := NewRentalService(testEquipment, testRentals, PolicyFunc(func(context.Context, Equipment, int, int) (bool, error) { return false, nil }))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := denied.Rent(testContext, rentalRequest(e)); !errors.Is(err, ErrConflict) {
		t.Fatalf("Очікується відмова, отримано %v", err)
	}
	if len(activeFor(t, e.ID)) != 0 {
		t.Fatal("Створено зайву оренду")
	}
}
func TestRepeatedReturn(t *testing.T) {
	saved := rentFor(t, equipmentFor(t))
	r, err := testService.ReturnRental(testContext, saved.ID)
	returned := must(t, r, err)
	if _, err = testService.ReturnRental(testContext, saved.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("Очікується відмова, отримано %v", err)
	}
	if stored(t, saved.ID) != returned {
		t.Fatal("Збережену оренду змінено")
	}
}

func TestSmoke(t *testing.T) {
	t.Run("CreateRental", TestCreateRental)
	t.Run("ReturnRental", TestReturnRental)
	t.Run("RentalAmount", TestRentalAmount)
	t.Run("StockRejection", TestStockRejection)
}
