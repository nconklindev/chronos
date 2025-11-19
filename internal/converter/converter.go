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

const RowDetectionLimit = 10

// DecimalToTime converts decimal hours to hh:mm format
func DecimalToTime(decimal float64) string {
	if decimal < 0 {
		return "00:00"
	}

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
		for j := 0; j < len(data.Rows) && j < RowDetectionLimit; j++ {
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
func ConvertCSV(inputFile, outputFile string, columnIndices []int, keepOriginal bool, progressChan chan<- float64) (*types.ConversionResult, error) {
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

	// We need to reconstruct the records with new columns if keepOriginal is true
	var newRecords [][]string

	totalRows := len(records)
	// If keepOriginal, we iterate through all records.
	// If not, we iterate from index 1.
	if keepOriginal {
		for i, record := range records {
			// Report progress
			if progressChan != nil {
				select {
				case progressChan <- float64(i) / float64(totalRows):
				default:
				}
			}

			var newRow []string
			for colIdx, cell := range record {
				newRow = append(newRow, cell)
				if colMap[colIdx] {
					// This is a column we are converting.
					// If it's the header row (i==0), append the new header
					if i == 0 {
						newRow = append(newRow, cell+" (HH:MM)")
					} else {
						// It's a data row. Calculate the converted value.
						val := strings.TrimSpace(cell)
						convertedVal := ""
						if val != "" {
							if decimal, err := strconv.ParseFloat(val, 64); err == nil {
								convertedVal = DecimalToTime(decimal)
							}
						}
						newRow = append(newRow, convertedVal)
					}
				}
			}
			newRecords = append(newRecords, newRow)
		}
		records = newRecords
	} else {
		// replace in place
		for i := 1; i < len(records); i++ {
			// Report progress
			if progressChan != nil {
				select {
				case progressChan <- float64(i) / float64(totalRows):
				default:
				}
			}

			for colIdx := range colMap {
				if colIdx < len(records[i]) {
					val := strings.TrimSpace(records[i][colIdx])
					if val != "" {
						if decimal, err := strconv.ParseFloat(val, 64); err == nil {
							records[i][colIdx] = DecimalToTime(decimal)
						}
					}
				}
			}
		}
	}

	// Count processed rows (excluding header)
	rowsProcessed := len(records) - 1

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
func ConvertXLSX(inputFile, outputFile string, columnIndices []int, keepOriginal bool, progressChan chan<- float64) (*types.ConversionResult, error) {
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

	headerRowIdx := findHeaderRow(rows)
	if headerRowIdx == -1 {
		return nil, fmt.Errorf("could not find header row")
	}

	headers := rows[headerRowIdx]
	colMap := make(map[int]bool)
	var convertedCols []string

	// Let's identify which columns to convert first.
	for _, idx := range columnIndices {
		if idx >= 0 && idx < len(headers) {
			colMap[idx] = true
			convertedCols = append(convertedCols, headers[idx])
		}
	}

	rowsProcessed := 0
	totalRows := len(rows) - (headerRowIdx + 2) + 1
	if totalRows < 0 {
		totalRows = 0
	}

	// Helper to report progress
	reportProgress := func(current int) {
		if progressChan != nil && totalRows > 0 {
			select {
			case progressChan <- float64(current) / float64(totalRows):
			default:
			}
		}
	}

	if keepOriginal {
		// Find max col index
		maxCol := len(headers) - 1

		// We need to iterate through all columns from right to left
		// If a column is in colMap, we insert a column after it.

		processedOps := 0
		totalOps := 0
		for colIdx := maxCol; colIdx >= 0; colIdx-- {
			if colMap[colIdx] {
				totalOps += totalRows
			}
		}

		for colIdx := maxCol; colIdx >= 0; colIdx-- {
			if colMap[colIdx] {
				// Insert column after this one (at colIdx + 2 because Excel is 1-indexed and we want after)
				// Just get the column name for the insertion point
				insertPoint, _ := excelize.CoordinatesToCellName(colIdx+2, 1)
				insertColLetter := strings.Map(func(r rune) rune {
					if r >= '0' && r <= '9' {
						return -1
					}
					return r
				}, insertPoint)

				if err := f.InsertCols(sheetName, insertColLetter, 1); err != nil {
					return nil, err
				}

				// Set header for new column
				headerCell, _ := excelize.CoordinatesToCellName(colIdx+2, headerRowIdx+1)
				f.SetCellValue(sheetName, headerCell, headers[colIdx]+" (HH:MM)")

				// Process rows for this column
				for rowIdx := headerRowIdx + 2; rowIdx <= len(rows); rowIdx++ {
					// Read original value
					origCell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx)
					val, _ := f.GetCellValue(sheetName, origCell)

					if val != "" {
						if decimal, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil {
							// Write to new column
							destCell, _ := excelize.CoordinatesToCellName(colIdx+2, rowIdx)
							f.SetCellValue(sheetName, destCell, DecimalToTime(decimal))
							rowsProcessed++
						}
					}

					processedOps++
					if progressChan != nil && totalOps > 0 {
						select {
						case progressChan <- float64(processedOps) / float64(totalOps):
						default:
						}
					}
				}
			}
		}
	} else {
		// Original behavior
		current := 0
		for rowIdx := headerRowIdx + 2; rowIdx <= len(rows); rowIdx++ {
			current++
			reportProgress(current)

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

	// Find the header row (first row with multiple non-empty cells)
	headerRowIdx := findHeaderRow(rows)
	if headerRowIdx == -1 {
		return nil, fmt.Errorf("could not find header row")
	}

	return &types.FileData{
		Headers:   rows[headerRowIdx],
		Rows:      rows[headerRowIdx+1:],
		HeaderRow: headerRowIdx,
	}, nil
}

// findHeaderRow locates the first row that appears to be a header
// by finding the row with the most non-empty text cells
func findHeaderRow(rows [][]string) int {
	maxNonEmpty := 0
	headerIdx := -1

	// Look at first 20 rows max
	searchLimit := len(rows)
	if searchLimit > RowDetectionLimit*2 {
		searchLimit = RowDetectionLimit * 2
	}

	for i := 0; i < searchLimit; i++ {
		nonEmptyCount := 0
		hasText := false

		for _, cell := range rows[i] {
			trimmed := strings.TrimSpace(cell)
			if trimmed != "" {
				nonEmptyCount++
				// Check if cell contains actual text (not just numbers or symbols)
				if containsLetters(trimmed) {
					hasText = true
				}
			}
		}

		// Header should have multiple columns AND contain text
		if nonEmptyCount >= 2 && hasText && nonEmptyCount > maxNonEmpty {
			maxNonEmpty = nonEmptyCount
			headerIdx = i
		}
	}

	return headerIdx
}

// containsLetters checks if a string contains any alphabetic characters
func containsLetters(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}
