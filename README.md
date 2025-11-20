# ⏰ Chronos

<img width="1171" height="325" alt="chronos" src="https://github.com/user-attachments/assets/ea9ff1c6-2d7c-4435-8411-280dc0834f5c" />

> A beautiful TUI application for converting decimal hours to HH:MM format in CSV and XLSX files.

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

## 🌟 Features

- **Interactive TUI** - Beautiful terminal user interface built with [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- **File Browser** - Browse and select files from your filesystem
- **Auto-Detection** - Automatically identifies columns containing decimal hours
- **Flexible Selection** - Choose which columns to convert
- **Multiple Formats** - Supports both CSV and XLSX files
- **Smart Conversion** - Converts decimal hours (e.g., 7.5) to HH:MM format (07:30)
- **Responsive Design** - Adapts to your terminal size

## 📦 Installation

### Download Pre-built Binaries

Download the latest release for your platform from the [releases page](https://github.com/nconklindev/chronos/releases).

#### Linux

```bash
curl -LO https://github.com/nconklindev/chronos/releases/latest/download/chronos_Linux_x86_64.tar.gz
tar -xzf chronos_Linux_x86_64.tar.gz
sudo mv chronos /usr/local/bin/
```

#### macOS

```bash
curl -LO https://github.com/nconklindev/chronos/releases/latest/download/chronos_Darwin_x86_64.tar.gz
tar -xzf chronos_Darwin_x86_64.tar.gz
sudo mv chronos /usr/local/bin/
```

##### 🍺 Homebrew

```bash
brew install --cask nconklindev/tap/chronos

## 🚀 Usage

Simply run the application:

```bash
chronos
```

### Workflow

1. **Select File** - Browse your filesystem and select up to 3 CSV or XLSX files to convert (can include CSV and XLSX in the same batch)
2. **Choose Columns** - Select which columns contain decimal hours (auto-detected by default)
3. **Convert** - Press Enter to convert and save the file

### Keyboard Controls

#### File Picker

- `↑/↓` or `k/j` - Navigate files and directories
- `Space` - Select file
- `Enter` - Confirm selection
- `q` - Quit

#### Column Selection

- `↑/↓` or `k/j` - Navigate columns
- `Space` - Toggle column selection
- `a` - Select all auto-detected columns
- `o` - Toggle keep original file columns
- `Enter` - Start conversion
- `q` - Quit

## 📝 Examples

### Input (CSV/XLSX)

```csv
Employee,Date,Regular Hours,Overtime Hours
John Doe,2025-10-10,8.5,1.75
Jane Smith,2025-10-10,7.25,0.5
```

### Output

```csv
Employee,Date,Regular Hours,Overtime Hours
John Doe,2025-10-10,08:30,01:45
Jane Smith,2025-10-10,07:15,00:30
```

### Conversion Examples

- `1.5` → `01:30`
- `7.75` → `07:45`
- `8.983` → `08:59` (rounds to nearest minute)
- `10.0` → `10:00`

## 🛠️ Development

### Prerequisites

- Go 1.21 or higher
- Make (optional)

### Setup

```bash
# Clone the repository
git clone https://github.com/nconklindev/chronos.git
cd chronos

# Install dependencies
go mod download

# Run the application
go run main.go

# Run tests
go test ./...
```

### Building

```bash
# Build for current platform
go build -o chronos

# Build for all platforms (requires goreleaser)
goreleaser build --snapshot --clean
```

Using goreleaser is optional, but recommended for building for multiple platforms. See [goreleaser](https://goreleaser.com/) for more information.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Terminal UI framework
- Styled with [Lipgloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
- Uses [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components
- Excel support via [Excelize](https://github.com/xuri/excelize)

## 📬 Contact

[@nconklindev](https://github.com/nconklindev)

Project Link: [https://github.com/nconklindev/chronos](https://github.com/nconklindev/chronos)

---

Made with ❤️ and ☕
