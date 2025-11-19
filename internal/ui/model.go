package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nconklindev/chronos/internal/converter"
	"github.com/nconklindev/chronos/internal/types"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type state int

const (
	stateFilePicker state = iota
	stateColumnSelection
	stateProcessing
	stateComplete
	stateError
)

type Model struct {
	state        state
	filepicker   filepicker.Model
	selectedFile string
	fileData     *types.FileData
	detectedCols []int
	selectedCols map[int]bool
	keepOriginal bool
	cursor       int
	result       *types.ConversionResult
	err          error
	width        int
	height       int
	progress     progress.Model
	progressChan chan float64
	resultChan   chan conversionResultMsg
}

type conversionResultMsg struct {
	result *types.ConversionResult
	err    error
}

type fileLoadedMsg struct {
	data *types.FileData
	err  error
}

type conversionCompleteMsg struct {
	result *types.ConversionResult
	err    error
}

type progressMsg float64

type waitForProgressMsg struct{}

func InitialModel() Model {
	fp := filepicker.New()
	fp.AllowedTypes = []string{".csv", ".xlsx"}
	fp.CurrentDirectory, _ = os.Getwd()

	// Set filepicker colors to match theme
	fp.Styles.Cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8C42"))
	fp.Styles.Symlink = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB84D"))
	fp.Styles.Directory = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB84D"))
	fp.Styles.File = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	fp.Styles.Permission = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	fp.Styles.Selected = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8C42")).Bold(true)
	fp.Styles.FileSize = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))

	// Initialize progress bar
	prog := progress.New(progress.WithGradient("#FF8C42", "#FF9F5A"))

	return Model{
		state:        stateFilePicker,
		filepicker:   fp,
		selectedCols: make(map[int]bool),
		progress:     prog,
	}
}

func (m Model) Init() tea.Cmd {
	return m.filepicker.Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Set filepicker height based on available space
		// Subtract space for title, subtitle, help text, and padding
		height := msg.Height - 14
		if height < 5 {
			height = 5 // Minimum height
		}

		m.filepicker.SetHeight(height)

		return m, nil

	case tea.KeyMsg:
		switch m.state {
		case stateFilePicker:
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			}

		case stateColumnSelection:
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < len(m.fileData.Headers)-1 {
					m.cursor++
				}
			case " ":
				m.selectedCols[m.cursor] = !m.selectedCols[m.cursor]
			case "o":
				m.keepOriginal = !m.keepOriginal
			case "a":
				// Select all detected columns
				for _, idx := range m.detectedCols {
					m.selectedCols[idx] = true
				}
			case "enter":
				if len(m.selectedCols) > 0 {
					m.state = stateProcessing
					return m.convertFile()
				}
			}

		case stateComplete, stateError:
			switch msg.String() {
			case "ctrl+c", "q", "enter", "esc":
				return m, tea.Quit
			}
		}

	case fileLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = stateError
			return m, nil
		}
		m.fileData = msg.data
		m.detectedCols = converter.AutoDetectColumns(msg.data)

		// Auto-select detected columns
		for _, idx := range m.detectedCols {
			m.selectedCols[idx] = true
		}

		m.state = stateColumnSelection
		return m, nil

	case conversionCompleteMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = stateError
			return m, nil
		}
		m.result = msg.result
		m.state = stateComplete
		return m, nil

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd

	case progressMsg:
		if m.state == stateProcessing {
			cmd := m.progress.SetPercent(float64(msg))
			return m, tea.Batch(cmd, waitForProgress(m.progressChan, m.resultChan))
		}
		return m, nil

	case waitForProgressMsg:
		return m, waitForProgress(m.progressChan, m.resultChan)
	}

	// Handle filepicker updates
	if m.state == stateFilePicker {
		var cmd tea.Cmd
		m.filepicker, cmd = m.filepicker.Update(msg)

		if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
			m.selectedFile = path
			return m, m.loadFile(path)
		}

		return m, cmd
	}

	return m, nil
}

func (m Model) loadFile(path string) tea.Cmd {
	return func() tea.Msg {
		data, err := converter.ReadFileData(path)
		return fileLoadedMsg{data: data, err: err}
	}
}

func (m Model) convertFile() (Model, tea.Cmd) {
	m.progressChan = make(chan float64, 100)
	m.resultChan = make(chan conversionResultMsg, 1)

	cmd := tea.Batch(
		func() tea.Msg {
			var selectedIndices []int
			for idx := range m.selectedCols {
				if m.selectedCols[idx] {
					selectedIndices = append(selectedIndices, idx)
				}
			}

			ext := strings.ToLower(filepath.Ext(m.selectedFile))
			base := strings.TrimSuffix(m.selectedFile, ext)
			outputFile := base + "_converted" + ext

			// Capture channels for the goroutine
			progressChan := m.progressChan
			resultChan := m.resultChan
			selectedFile := m.selectedFile
			keepOriginal := m.keepOriginal

			go func() {
				var result *types.ConversionResult
				var err error

				switch ext {
				case ".csv":
					result, err = converter.ConvertCSV(selectedFile, outputFile, selectedIndices, keepOriginal, progressChan)
				case ".xlsx":
					result, err = converter.ConvertXLSX(selectedFile, outputFile, selectedIndices, keepOriginal, progressChan)
				}

				// Send result
				resultChan <- conversionResultMsg{result: result, err: err}

				// Close channels
				close(progressChan)
				close(resultChan)
			}()

			return waitForProgressMsg{}
		},
		waitForProgress(m.progressChan, m.resultChan),
		m.progress.Init(), // Start progress bar animation
	)

	return m, cmd
}

func waitForProgress(progressChan chan float64, resultChan chan conversionResultMsg) tea.Cmd {
	return func() tea.Msg {
		if progressChan == nil {
			return nil
		}

		p, ok := <-progressChan
		if !ok {
			// Progress channel closed, check result
			res, ok := <-resultChan
			if ok {
				return conversionCompleteMsg(res)
			}
			return nil
		}

		return progressMsg(p)
	}
}

func (m Model) View() string {
	switch m.state {
	case stateFilePicker:
		return m.viewFilePicker()
	case stateColumnSelection:
		return m.viewColumnSelection()
	case stateProcessing:
		return m.viewProcessing()
	case stateComplete:
		return m.viewComplete()
	case stateError:
		return m.viewError()
	}
	return ""
}

func (m Model) viewFilePicker() string {
	var s strings.Builder

	title := TitleStyle.Render("⏰ Chronos - Decimal to Hour Converter")

	authorSpan := SubtitleStyle.Render("by Nick Conklin • ")
	githubSpan := LinkStyle.Render("https://github.com/nconklindev/chronos")
	byLine := lipgloss.JoinHorizontal(lipgloss.Top, authorSpan, githubSpan)

	s.WriteString(lipgloss.JoinVertical(lipgloss.Left, title, byLine))
	s.WriteString("\n")
	s.WriteString(SubtitleStyle.Render("Select a CSV or XLSX file to convert"))
	s.WriteString("\n\n")
	s.WriteString(m.filepicker.View())
	s.WriteString("\n\n")
	s.WriteString(HelpStyle.Render("Press q to quit"))

	return s.String()
}

func (m Model) viewColumnSelection() string {
	var s strings.Builder

	s.WriteString(TitleStyle.Render("⏰ Select Columns to Convert"))
	s.WriteString("\n")
	s.WriteString(SubtitleStyle.Render(fmt.Sprintf("File: %s", filepath.Base(m.selectedFile))))
	s.WriteString("\n\n")

	if len(m.detectedCols) > 0 {
		s.WriteString(SuccessStyle.Render(fmt.Sprintf("✓ Auto-detected %d decimal hour column(s)", len(m.detectedCols))))
		s.WriteString("\n\n")
	}

	for i, header := range m.fileData.Headers {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}

		checked := " "
		if m.selectedCols[i] {
			checked = "✓"
		}

		line := fmt.Sprintf("%s [%s] %s", cursor, checked, header)

		isDetected := false
		for _, idx := range m.detectedCols {
			if idx == i {
				isDetected = true
				break
			}
		}

		if m.cursor == i {
			line = SelectedStyle.Render(line)
		} else if m.selectedCols[i] {
			line = CheckedStyle.Render(line)
		} else if isDetected {
			line = UnselectedStyle.Render(line + " (detected)")
		}

		s.WriteString(line)
		s.WriteString("\n")
	}

	s.WriteString("\n")

	keepOriginalStatus := "[ ]"
	if m.keepOriginal {
		keepOriginalStatus = "[x]"
	}
	s.WriteString(fmt.Sprintf("Keep Original Columns: %s\n", keepOriginalStatus))
	s.WriteString("\n")
	s.WriteString(HelpStyle.Render("↑/↓: navigate • space: toggle • o: keep original • a: select all detected • enter: convert • q: quit"))

	return BoxStyle.Render(s.String())
}

func (m Model) viewProcessing() string {
	var s strings.Builder

	s.WriteString(TitleStyle.Render("⏰ Processing..."))
	s.WriteString("\n\n")
	s.WriteString("Converting decimal hours to HH:MM format...")
	s.WriteString("\n\n")
	s.WriteString(m.progress.View())

	return BoxStyle.Render(s.String())
}

func (m Model) viewComplete() string {
	var s strings.Builder

	s.WriteString(TitleStyle.Render("✓ Conversion Complete!"))
	s.WriteString("\n\n")

	// Truncate paths if they're too long
	maxPathLen := m.width - 20 // Leave room for padding and borders
	if maxPathLen < 30 {
		maxPathLen = 30
	}

	inputPath := m.result.InputFile
	if len(inputPath) > maxPathLen {
		inputPath = "..." + inputPath[len(inputPath)-maxPathLen+3:]
	}

	outputPath := m.result.OutputFile
	if len(outputPath) > maxPathLen {
		outputPath = "..." + outputPath[len(outputPath)-maxPathLen+3:]
	}

	s.WriteString(fmt.Sprintf("Input:  %s\n", inputPath))
	s.WriteString(SuccessStyle.Render(fmt.Sprintf("Output: %s\n", outputPath)))
	s.WriteString("\n")
	s.WriteString(fmt.Sprintf("Columns converted: %s\n", strings.Join(m.result.ColumnsFound, ", ")))
	s.WriteString(fmt.Sprintf("Values processed: %d\n", m.result.RowsProcessed))
	s.WriteString("\n")
	s.WriteString(HelpStyle.Render("Press any key to exit"))

	return BoxStyle.Render(s.String())
}

func (m Model) viewError() string {
	var s strings.Builder

	s.WriteString(ErrorStyle.Render("✗ Error"))
	s.WriteString("\n\n")
	s.WriteString(m.err.Error())
	s.WriteString("\n\n")
	s.WriteString(HelpStyle.Render("Press any key to exit"))

	return BoxStyle.Render(s.String())
}
