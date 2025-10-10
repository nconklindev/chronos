package types

type ConversionResult struct {
	InputFile     string
	OutputFile    string
	ColumnsFound  []string
	RowsProcessed int
}

type FileData struct {
	Headers []string
	Rows    [][]string
}
