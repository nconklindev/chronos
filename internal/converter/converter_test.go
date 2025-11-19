package converter

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"

	"github.com/nconklindev/chronos/internal/types"
)

func TestDecimalToTime(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected string
	}{
		{"Zero", 0, "00:00"},
		{"Half hour", 0.5, "00:30"},
		{"One hour", 1.0, "01:00"},
		{"One and a half hours", 1.5, "01:30"},
		{"Quarter hour", 0.25, "00:15"},
		{"Three quarters hour", 0.75, "00:45"},
		{"Rounding down", 1.01, "01:01"}, // 0.01 * 60 = 0.6 -> 1
		{"Rounding up", 1.99, "01:59"},   // 0.99 * 60 = 59.4 -> 59
		{"Exact minute", 1.0 + 1.0/60.0, "01:01"},
		{"Large number", 123.45, "123:27"},
		{"Negative number", -1.5, "00:00"}, // Should be clamped to 0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecimalToTime(tt.input)
			if got != tt.expected {
				t.Errorf("DecimalToTime(%f) = %s; want %s", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIsDecimalHour(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Valid integer", "1", true},
		{"Valid decimal", "1.5", true},
		{"Valid zero", "0", true},
		{"Empty string", "", false},
		{"Whitespace", "   ", false},
		{"Non-numeric", "abc", false},
		{"Negative", "-1", false},
		{"Too large", "10000", false},
		{"Mixed", "1.5h", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDecimalHour(tt.input)
			if got != tt.expected {
				t.Errorf("IsDecimalHour(%q) = %v; want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestAutoDetectColumns(t *testing.T) {
	tests := []struct {
		name     string
		data     *types.FileData
		expected []int
	}{
		{
			name: "Detects single column",
			data: &types.FileData{
				Headers: []string{"Name", "Hours"},
				Rows: [][]string{
					{"Alice", "8.0"},
					{"Bob", "7.5"},
				},
			},
			expected: []int{1},
		},
		{
			name: "Detects multiple columns",
			data: &types.FileData{
				Headers: []string{"Regular", "Overtime"},
				Rows: [][]string{
					{"8.0", "1.5"},
					{"7.5", "0.5"},
				},
			},
			expected: []int{0, 1},
		},
		{
			name: "Ignores non-decimal columns",
			data: &types.FileData{
				Headers: []string{"Name", "ID"},
				Rows: [][]string{
					{"Alice", "ID-123"},
					{"Bob", "ID-456"},
				},
			},
			expected: nil,
		},
		{
			name: "Handles empty rows",
			data: &types.FileData{
				Headers: []string{"Hours"},
				Rows: [][]string{
					{""},
					{"8.0"},
				},
			},
			expected: []int{0},
		},
		{
			name: "Respects row limit",
			data: &types.FileData{
				Headers: []string{"Mixed"},
				Rows: [][]string{
					{"8.0"},
					{"7.5"},
					{"Invalid"}, // Should be caught if within limit
				},
			},
			// Assuming limit checks first 10 rows, this should fail detection
			// because "Invalid" is not a decimal hour
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AutoDetectColumns(tt.data)
			if len(got) != len(tt.expected) {
				t.Errorf("AutoDetectColumns() = %v; want %v", got, tt.expected)
				return
			}
			for i, v := range got {
				if v != tt.expected[i] {
					t.Errorf("AutoDetectColumns() = %v; want %v", got, tt.expected)
				}
			}
		})
	}
}

func TestConvertCSV_KeepOriginal(t *testing.T) {
	// Create temp input file
	tmpDir := t.TempDir()
	inputFile := filepath.Join(tmpDir, "input.csv")
	outputFile := filepath.Join(tmpDir, "output.csv")

	inputData := [][]string{
		{"Name", "Hours"},
		{"Alice", "1.5"},
		{"Bob", "2.0"},
	}

	f, err := os.Create(inputFile)
	if err != nil {
		t.Fatal(err)
	}
	w := csv.NewWriter(f)
	w.WriteAll(inputData)
	f.Close()

	// Test with keepOriginal = true
	_, err = ConvertCSV(inputFile, outputFile, []int{1}, true, nil)
	if err != nil {
		t.Fatalf("ConvertCSV failed: %v", err)
	}

	// Verify output
	f, err = os.Open(outputFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}

	if len(records) != 3 {
		t.Errorf("Expected 3 records, got %d", len(records))
	}

	// Check headers
	expectedHeaders := []string{"Name", "Hours", "Hours (HH:MM)"}
	if len(records[0]) != 3 {
		t.Errorf("Expected 3 columns, got %d", len(records[0]))
	}
	for i, h := range expectedHeaders {
		if records[0][i] != h {
			t.Errorf("Header %d: expected %s, got %s", i, h, records[0][i])
		}
	}

	// Check data
	if records[1][2] != "01:30" {
		t.Errorf("Expected 01:30, got %s", records[1][2])
	}
	if records[2][2] != "02:00" {
		t.Errorf("Expected 02:00, got %s", records[2][2])
	}
}
