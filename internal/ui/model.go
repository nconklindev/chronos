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
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type state int

const (
	// stateFilePicker is the initial state where the user selects files to convert.
	stateFilePicker state = iota
	// stateLoading is a transitional state while a file is being read from disk.
	stateLoading
	// stateColumnSelection is where the user configures which columns to convert for a specific file.
	stateColumnSelection
	// stateProcessing indicates that the conversion process is running.
	stateProcessing
	// stateComplete is the final state showing the results of the conversion.
	stateComplete
	// stateError displays any errors that occurred during the process.
	stateError
)

type fileConfig struct {
	path              string
	fileData          *types.FileData
	detectedCols      []int
	selectedCols      map[int]bool
	selectableIndices []int
	keepOriginal      bool
	cursor            int
}

// Model holds the application state.
type Model struct {
	state      state
	filepicker filepicker.Model
	viewport   viewport.Model

	// selectedFiles stores the paths of all files selected by the user.
	selectedFiles []string
	// currentFileIndex tracks which file is currently being configured or processed.
	currentFileIndex int
	// configs holds the column selection and settings for each selected file.
	configs []fileConfig
	// results stores the outcome of each file conversion.
	results []*types.ConversionResult

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
	fp.CurrentDirectory, _ = os.UserHomeDir()

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
		state:         stateFilePicker,
		filepicker:    fp,
		selectedFiles: []string{},
		configs:       []fileConfig{},
		progress:      prog,
		viewport:      viewport.New(0, 0),
	}
}

func (m Model) Init() tea.Cmd {
	return m.filepicker.Init()
}

// Update handles incoming events and updates the model state.
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

		// Update viewport height
		// Header is approx 7 lines, footer is approx 5 lines + borders/padding
		// Total chrome is approx 16 lines
		vpHeight := msg.Height - 16
		if vpHeight < 5 {
			vpHeight = 5
		}
		m.viewport.Width = msg.Width - 4 // Account for padding
		m.viewport.Height = vpHeight

		// If we are in column selection, update content to ensure it fits
		if m.state == stateColumnSelection {
			m.updateViewportContent()
		}

		return m, nil

	case tea.KeyMsg:
		switch m.state {
		case stateFilePicker:
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case " ":
				// Spacebar is used to select a file. We simulate an Enter keypress
				// for the filepicker component to trigger its selection logic.
				enterMsg := tea.KeyMsg{Type: tea.KeyEnter}

				var cmd tea.Cmd
				m.filepicker, cmd = m.filepicker.Update(enterMsg)

				if didSelect, path := m.filepicker.DidSelectFile(enterMsg); didSelect {
					// Check if file is already selected
					alreadySelected := false
					for _, p := range m.selectedFiles {
						if p == path {
							alreadySelected = true
							break
						}
					}

					if !alreadySelected && len(m.selectedFiles) < 3 {
						m.selectedFiles = append(m.selectedFiles, path)
					}
					return m, nil
				}
				return m, cmd
			case "enter":
				// Enter confirms the selection of all files and proceeds to the next step.
				if len(m.selectedFiles) > 0 {
					// Start loading the first file to prepare for column selection.
					m.currentFileIndex = 0
					m.state = stateLoading
					return m, m.loadFile(m.selectedFiles[0])
				}
			case "backspace", "delete":
				if len(m.selectedFiles) > 0 {
					m.selectedFiles = m.selectedFiles[:len(m.selectedFiles)-1]
				}
			}

		case stateColumnSelection:
			config := &m.configs[m.currentFileIndex]
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "up", "k":
				if config.cursor > 0 {
					config.cursor--
					if config.cursor < m.viewport.YOffset {
						m.viewport.SetYOffset(config.cursor)
					}
					m.updateViewportContent()
				}
			case "down", "j":
				if config.cursor < len(config.selectableIndices)-1 {
					config.cursor++
					if config.cursor >= m.viewport.YOffset+m.viewport.Height {
						m.viewport.SetYOffset(config.cursor - m.viewport.Height + 1)
					}
					m.updateViewportContent()
				}
			case " ":
				// Toggle selection for the column at the current cursor position
				colIdx := config.selectableIndices[config.cursor]
				config.selectedCols[colIdx] = !config.selectedCols[colIdx]
				m.updateViewportContent()
			case "o":
				config.keepOriginal = !config.keepOriginal
				m.updateViewportContent()
			case "a":
				// Select all detected columns
				for _, idx := range config.detectedCols {
					config.selectedCols[idx] = true
				}
				m.updateViewportContent()
			case "enter":
				if len(config.selectedCols) > 0 {
					// If there are more files to configure, load the next one.
					if m.currentFileIndex < len(m.selectedFiles)-1 {
						m.currentFileIndex++
						m.state = stateLoading
						return m, m.loadFile(m.selectedFiles[m.currentFileIndex])
					} else {
						// All files configured, start the batch conversion process.
						m.state = stateProcessing
						m.currentFileIndex = 0 // Reset index to start processing from the first file.
						return m.convertNextFile()
					}
				}
			}

		case stateComplete, stateError:
			switch msg.String() {
			case "ctrl+c", "q", "esc":
				return m, tea.Quit
			case "enter":
				// Reset to initial state
				m.state = stateFilePicker
				m.selectedFiles = []string{}
				m.configs = []fileConfig{}
				m.results = []*types.ConversionResult{}
				m.currentFileIndex = 0
				m.err = nil
				return m, nil
			}
		}

	// fileLoadedMsg is received when a file has been read from disk.
	case fileLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = stateError
			return m, nil
		}

		// Auto-detect columns that look like decimal hours.
		detected := converter.AutoDetectColumns(msg.data)
		selected := make(map[int]bool)
		for _, idx := range detected {
			selected[idx] = true
		}

		// Filter out empty headers
		var selectable []int
		for i, header := range msg.data.Headers {
			if strings.TrimSpace(header) != "" {
				selectable = append(selectable, i)
			}
		}

		// Create a configuration for this file.
		config := fileConfig{
			path:              m.selectedFiles[m.currentFileIndex],
			fileData:          msg.data,
			detectedCols:      detected,
			selectedCols:      selected,
			selectableIndices: selectable,
			keepOriginal:      false,
			cursor:            0,
		}

		// Ensure configs slice is large enough
		if len(m.configs) <= m.currentFileIndex {
			m.configs = append(m.configs, config)
		} else {
			m.configs[m.currentFileIndex] = config
		}

		m.state = stateColumnSelection

		// Reset viewport scroll and update content
		m.viewport.SetYOffset(0)
		m.updateViewportContent()

		return m, nil

	// conversionCompleteMsg is received when a single file conversion finishes.
	case conversionCompleteMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = stateError
			return m, nil
		}
		m.results = append(m.results, msg.result)

		// If there are more files in the queue, start converting the next one.
		if m.currentFileIndex < len(m.selectedFiles)-1 {
			m.currentFileIndex++
			return m.convertNextFile()
		}

		// All files processed.
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
		return m, cmd
	}

	return m, nil
}

// loadFile reads the file content asynchronously.
func (m Model) loadFile(path string) tea.Cmd {
	return func() tea.Msg {
		data, err := converter.ReadFileData(path)
		return fileLoadedMsg{data: data, err: err}
	}
}

// convertNextFile starts the conversion process for the current file in the queue.
func (m Model) convertNextFile() (Model, tea.Cmd) {
	m.progressChan = make(chan float64, 100)
	m.resultChan = make(chan conversionResultMsg, 1)

	config := m.configs[m.currentFileIndex]

	cmd := tea.Batch(
		func() tea.Msg {
			var selectedIndices []int
			for idx := range config.selectedCols {
				if config.selectedCols[idx] {
					selectedIndices = append(selectedIndices, idx)
				}
			}

			ext := strings.ToLower(filepath.Ext(config.path))
			base := strings.TrimSuffix(config.path, ext)
			outputFile := base + "_converted" + ext

			// Capture channels for the goroutine
			progressChan := m.progressChan
			resultChan := m.resultChan
			selectedFile := config.path
			keepOriginal := config.keepOriginal

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
	case stateLoading:
		return m.viewLoading()
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
	s.WriteString(SubtitleStyle.Render("Select up to 3 files to convert"))
	s.WriteString("\n\n")

	// Show selected files
	if len(m.selectedFiles) > 0 {
		s.WriteString("Selected Files:\n")
		for i, file := range m.selectedFiles {
			s.WriteString(fmt.Sprintf("%d. %s\n", i+1, filepath.Base(file)))
		}
		s.WriteString("\n")
		if len(m.selectedFiles) < 3 {
			s.WriteString(SubtitleStyle.Render(fmt.Sprintf("(%d/3 selected) Select more or press 'c' to continue", len(m.selectedFiles))))
		} else {
			s.WriteString(SuccessStyle.Render("Max files selected. Press 'c' to continue."))
		}
		s.WriteString("\n\n")
	}

	s.WriteString(m.filepicker.View())
	s.WriteString("\n\n")
	s.WriteString(HelpStyle.Render("Space: select file • Enter: confirm selection • Backspace: remove last file • q: quit"))

	return s.String()
}

func (m Model) viewColumnSelection() string {
	var s strings.Builder
	config := m.configs[m.currentFileIndex]

	s.WriteString(TitleStyle.Render("⏰ Select Columns to Convert"))
	s.WriteString("\n")
	s.WriteString(SubtitleStyle.Render(fmt.Sprintf("File (%d/%d): %s", m.currentFileIndex+1, len(m.selectedFiles), filepath.Base(config.path))))
	s.WriteString("\n\n")

	if len(config.detectedCols) > 0 {
		s.WriteString(SuccessStyle.Render(fmt.Sprintf("✓ Auto-detected %d decimal hour column(s)", len(config.detectedCols))))
		s.WriteString("\n\n")
	}

	visibleHeight := m.height - 20
	if visibleHeight < 5 {
		visibleHeight = 5
	}
	// Ensure viewport height is set (in case window size msg hasn't happened yet or logic differs)
	// We rely on Update to set it properly, but for safety we can check here or just use what's there.
	// The viewport.View() will use its internal height.

	s.WriteString(m.viewport.View())
	s.WriteString("\n\n")

	// Show scroll position indicator
	totalCols := len(config.selectableIndices)
	visibleStart := m.viewport.YOffset + 1
	visibleEnd := m.viewport.YOffset + m.viewport.Height
	if visibleEnd > totalCols {
		visibleEnd = totalCols
	}
	if visibleStart > totalCols {
		visibleStart = totalCols
	}
	scrollInfo := SubtitleStyle.Render(fmt.Sprintf("Viewing %d-%d of %d columns", visibleStart, visibleEnd, totalCols))
	s.WriteString(scrollInfo)
	s.WriteString("\n\n")

	keepOriginalStatus := "[ ]"
	if config.keepOriginal {
		keepOriginalStatus = "[x]"
	}
	s.WriteString(fmt.Sprintf("Keep Original Columns: %s\n", keepOriginalStatus))
	s.WriteString("\n")
	s.WriteString(HelpStyle.Render("↑/↓: navigate • space: toggle • o: keep original • a: select all detected • enter: confirm • q: quit"))

	return s.String()
}

func (m *Model) updateViewportContent() {
	if m.currentFileIndex >= len(m.configs) {
		return
	}
	config := m.configs[m.currentFileIndex]
	var s strings.Builder

	for i, colIdx := range config.selectableIndices {
		header := config.fileData.Headers[colIdx]
		cursor := " "
		if config.cursor == i {
			cursor = ">"
		}

		checked := " "
		if config.selectedCols[colIdx] {
			checked = "✓"
		}

		line := fmt.Sprintf("%s [%s] %s", cursor, checked, header)

		isDetected := false
		for _, idx := range config.detectedCols {
			if idx == colIdx {
				isDetected = true
				break
			}
		}

		if config.cursor == i {
			line = SelectedStyle.Render(line)
		} else if config.selectedCols[colIdx] {
			line = CheckedStyle.Render(line)
		} else if isDetected {
			line = UnselectedStyle.Render(line + " (detected)")
		}

		s.WriteString(line)
		s.WriteString("\n")
	}

	m.viewport.SetContent(s.String())
}

func (m Model) viewLoading() string {
	return BoxStyle.Render(TitleStyle.Render("Loading file..."))
}

func (m Model) viewProcessing() string {
	var s strings.Builder

	s.WriteString(TitleStyle.Render("⏰ Processing..."))
	s.WriteString("\n\n")
	s.WriteString(fmt.Sprintf("Converting file %d of %d...", m.currentFileIndex+1, len(m.selectedFiles)))
	s.WriteString("\n")
	s.WriteString(filepath.Base(m.configs[m.currentFileIndex].path))
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

	for _, res := range m.results {
		inputPath := res.InputFile
		if len(inputPath) > maxPathLen {
			inputPath = "..." + inputPath[len(inputPath)-maxPathLen+3:]
		}

		outputPath := res.OutputFile
		if len(outputPath) > maxPathLen {
			outputPath = "..." + outputPath[len(outputPath)-maxPathLen+3:]
		}

		s.WriteString(fmt.Sprintf("Input:    %s\n", inputPath))
		s.WriteString(SuccessStyle.Render(fmt.Sprintf("Output:   %s", outputPath)))
		s.WriteString("\n")
		s.WriteString(fmt.Sprintf("Columns:  %s", strings.Join(res.ColumnsFound, ", ")))
		s.WriteString("\n")
		s.WriteString(fmt.Sprintf("Rows:     %d", res.RowsProcessed))
		s.WriteString("\n")
		s.WriteString("---")
		s.WriteString("\n\n")
	}

	s.WriteString(HelpStyle.Render("Press Enter to convert more files or q to quit"))

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
