package rental

import (
	"errors"
	"testing"
)

func TestCalculateTotal(t *testing.T) {
	tests := []struct {
		name     string
		rate     int64
		quantity int
		days     int
		premium  bool
		want     int64
	}{
		{
			name:     "without_discount",
			rate:     10000,
			quantity: 2,
			days:     2,
			want:     40000,
		},
		{
			name:     "six_days_without_discount",
			rate:     10000,
			quantity: 1,
			days:     6,
			want:     60000,
		},
		{
			name:     "seven_days_discount",
			rate:     10000,
			quantity: 1,
			days:     7,
			want:     65000,
		},
		{
			name:     "thirteen_days_discount",
			rate:     10000,
			quantity: 1,
			days:     13,
			want:     120900,
		},
		{
			name:     "fourteen_days_discount",
			rate:     10000,
			quantity: 1,
			days:     14,
			want:     119000,
		},
		{
			name:     "discount_rounds_down",
			rate:     101,
			quantity: 1,
			days:     7,
			want:     658,
		},
		{
			name:     "premium_has_same_discount",
			rate:     10000,
			quantity: 1,
			days:     14,
			premium:  true,
			want:     119000,
		},
		{
			name:     "maximum_valid_values",
			rate:     1000000,
			quantity: 20,
			days:     30,
			want:     510000000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateTotal(
				tt.rate,
				tt.quantity,
				tt.days,
				tt.premium,
			)
			if err != nil {
				t.Fatalf("CalculateTotal повернув помилку: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Очікується %d, отримано %d", tt.want, got)
			}
		})
	}
}

func TestCalculateTotalInvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		rate     int64
		quantity int
		days     int
	}{
		{"rate_below_minimum", 99, 1, 1},
		{"rate_above_maximum", 1000001, 1, 1},
		{"quantity_below_minimum", 10000, 0, 1},
		{"quantity_above_maximum", 10000, 21, 1},
		{"days_below_minimum", 10000, 1, 0},
		{"days_above_maximum", 10000, 1, 31},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CalculateTotal(
				tt.rate,
				tt.quantity,
				tt.days,
				false,
			)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Очікується ErrInvalid, отримано %v", err)
			}
		})
	}
}
