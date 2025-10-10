package converter

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nconklindev/chronos/internal/types"

	"github.com/xuri/excelize/v2"
)

// DecimalToTime converts decimal hours to hh:mm format
func DecimalToTime(decimal float64) string {
	hours := int(decimal)
	minutesDecimal := (decimal - float64(hours)) * 60
	minutes := int(minutesDecimal + 0.5) // Round to nearest minute

	// Handle case where rounding minutes reaches 60
	if minutes >= 60 {
		hours++
		minutes -= 60
	}

	return fmt.Sprintf("%02d:%02d", hours, minutes)
}

// IsDecimalHour checks if a string looks like a decimal hour value
func IsDecimalHour(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}

	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return false
	}

	return val >= 0 && val < 10000
}

// AutoDetectColumns identifies columns that contain decimal hour values
func AutoDetectColumns(data *types.FileData) []int {
	var detectedIndices []int

	for i := range data.Headers {
		hasDecimalHours := true
		checkedRows := 0

		// Check first 10 data rows
		for j := 0; j < len(data.Rows) && j < 10; j++ {
			if i < len(data.Rows[j]) {
				val := strings.TrimSpace(data.Rows[j][i])
				if val != "" {
					if !IsDecimalHour(val) {
						hasDecimalHours = false
						break
					}
					checkedRows++
				}
			}
		}

		if hasDecimalHours && checkedRows > 0 {
			detectedIndices = append(detectedIndices, i)
		}
	}

	return detectedIndices
}

// ConvertCSV processes a CSV file and converts specified columns
func ConvertCSV(inputFile, outputFile string, columnIndices []int) (*types.ConversionResult, error) {
	// Read input file
	inFile, err := os.Open(inputFile)
	if err != nil {
		return nil, err
	}
	defer inFile.Close()

	reader := csv.NewReader(inFile)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("empty CSV file")
	}

	headers := records[0]
	colMap := make(map[int]bool)
	var convertedCols []string

	for _, idx := range columnIndices {
		if idx >= 0 && idx < len(headers) {
			colMap[idx] = true
			convertedCols = append(convertedCols, headers[idx])
		}
	}

	// Convert specified columns
	rowsProcessed := 0
	for i := 1; i < len(records); i++ {
		for colIdx := range colMap {
			if colIdx < len(records[i]) {
				val := strings.TrimSpace(records[i][colIdx])
				if val != "" {
					if decimal, err := strconv.ParseFloat(val, 64); err == nil {
						records[i][colIdx] = DecimalToTime(decimal)
						rowsProcessed++
					}
				}
			}
		}
	}

	// Write output file
	outFile, err := os.Create(outputFile)
	if err != nil {
		return nil, err
	}
	defer outFile.Close()

	writer := csv.NewWriter(outFile)
	defer writer.Flush()

	if err := writer.WriteAll(records); err != nil {
		return nil, err
	}

	return &types.ConversionResult{
		InputFile:     inputFile,
		OutputFile:    outputFile,
		ColumnsFound:  convertedCols,
		RowsProcessed: rowsProcessed,
	}, nil
}

// ConvertXLSX processes an XLSX file and converts specified columns
func ConvertXLSX(inputFile, outputFile string, columnIndices []int) (*types.ConversionResult, error) {
	f, err := excelize.OpenFile(inputFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sheetName := f.GetSheetName(0)
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("empty XLSX file")
	}

	headers := rows[0]
	colMap := make(map[int]bool)
	var convertedCols []string

	for _, idx := range columnIndices {
		if idx >= 0 && idx < len(headers) {
			colMap[idx] = true
			convertedCols = append(convertedCols, headers[idx])
		}
	}

	// Convert specified columns
	rowsProcessed := 0
	for rowIdx := 2; rowIdx <= len(rows); rowIdx++ {
		for colIdx := range colMap {
			cellName, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx)
			cellValue, _ := f.GetCellValue(sheetName, cellName)

			if cellValue != "" {
				if decimal, err := strconv.ParseFloat(strings.TrimSpace(cellValue), 64); err == nil {
					f.SetCellValue(sheetName, cellName, DecimalToTime(decimal))
					rowsProcessed++
				}
			}
		}
	}

	if err := f.SaveAs(outputFile); err != nil {
		return nil, err
	}

	return &types.ConversionResult{
		InputFile:     inputFile,
		OutputFile:    outputFile,
		ColumnsFound:  convertedCols,
		RowsProcessed: rowsProcessed,
	}, nil
}

// ReadFileData reads headers and sample rows from a file
func ReadFileData(filePath string) (*types.FileData, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".csv":
		return readCSVData(filePath)
	case ".xlsx":
		return readXLSXData(filePath)
	default:
		return nil, fmt.Errorf("unsupported file type: %s", ext)
	}
}

func readCSVData(filePath string) (*types.FileData, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("empty file")
	}

	return &types.FileData{
		Headers: records[0],
		Rows:    records[1:],
	}, nil
}

func readXLSXData(filePath string) (*types.FileData, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sheetName := f.GetSheetName(0)
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("empty file")
	}

	return &types.FileData{
		Headers: rows[0],
		Rows:    rows[1:],
	}, nil
}
