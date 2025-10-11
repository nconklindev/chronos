package types

type ConversionResult struct {
	InputFile     string
	OutputFile    string
	ColumnsFound  []string
	RowsProcessed int
}

type FileData struct {
	Headers   []string
	Rows      [][]string
	HeaderRow int // Which row the headers were found on in XLSX files (0-index)
}
