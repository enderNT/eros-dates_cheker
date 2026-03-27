package termlog

import (
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	colorReset  = "\033[0m"
	colorGray   = "\033[90m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
)

type Logger struct {
	mu        sync.Mutex
	base      *log.Logger
	useColors bool
}

func New(writer io.Writer) *Logger {
	if writer == nil {
		writer = os.Stdout
	}
	return &Logger{
		base:      log.New(writer, "", 0),
		useColors: shouldUseColors(),
	}
}

func (l *Logger) Section(title string, kv ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	line := strings.Repeat("=", 78)
	l.base.Println(l.paint(colorCyan, line))
	l.base.Println(l.paint(colorCyan, l.compose("BLOCK", title, kv...)))
	l.base.Println(l.paint(colorCyan, line))
}

func (l *Logger) RunStart(title string, kv ...any) {
	l.banner(colorCyan, "🚀", "RUN START", title, kv...)
}

func (l *Logger) RunEnd(title string, ok bool, kv ...any) {
	color := colorGreen
	icon := "✅"
	label := "RUN END"
	if !ok {
		color = colorYellow
		icon = "⚠️"
		label = "RUN END"
	}
	l.banner(color, icon, label, title, kv...)
}

func (l *Logger) Divider() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.base.Println(l.paint(colorGray, strings.Repeat("-", 78)))
}

func (l *Logger) Step(message string, kv ...any) {
	l.print("STEP", colorBlue, message, kv...)
}

func (l *Logger) Info(message string, kv ...any) {
	l.print("INFO", colorGray, message, kv...)
}

func (l *Logger) Success(message string, kv ...any) {
	l.print("OK", colorGreen, message, kv...)
}

func (l *Logger) Warn(message string, kv ...any) {
	l.print("WARN", colorYellow, message, kv...)
}

func (l *Logger) Error(message string, kv ...any) {
	l.print("ERROR", colorRed, message, kv...)
}

func (l *Logger) Table(title string, headers []string, rows [][]string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if title != "" {
		l.base.Println(l.paint(colorCyan, l.compose("TABLE", title)))
	}
	if len(headers) == 0 {
		l.base.Println(l.paint(colorGray, "(sin columnas)"))
		return
	}

	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}
	for _, row := range rows {
		for i := range min(len(row), len(widths)) {
			if len(row[i]) > widths[i] {
				widths[i] = len(row[i])
			}
		}
	}

	headerCells := make([]string, len(headers))
	for i, header := range headers {
		headerCells[i] = padRight(header, widths[i])
	}
	l.base.Println(l.paint(colorBlue, "┌"+joinTableBorder(widths, "┬")+"┐"))
	l.base.Println(l.paint(colorBlue, "│ "+strings.Join(headerCells, " │ ")+" │"))
	l.base.Println(l.paint(colorBlue, "├"+joinTableBorder(widths, "┼")+"┤"))
	if len(rows) == 0 {
		empty := make([]string, len(headers))
		for i := range empty {
			empty[i] = padRight("-", widths[i])
		}
		l.base.Println(l.paint(colorGray, "│ "+strings.Join(empty, " │ ")+" │"))
	} else {
		for _, row := range rows {
			cells := make([]string, len(headers))
			for i := range headers {
				value := ""
				if i < len(row) {
					value = row[i]
				}
				cells[i] = padRight(value, widths[i])
			}
			l.base.Println("│ " + strings.Join(cells, " │ ") + " │")
		}
	}
	l.base.Println(l.paint(colorBlue, "└"+joinTableBorder(widths, "┴")+"┘"))
}

func (l *Logger) print(level, color, message string, kv ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.base.Println(l.paint(color, l.compose(level, message, kv...)))
}

func (l *Logger) banner(color, icon, level, title string, kv ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	line := strings.Repeat("█", 90)
	l.base.Println(l.paint(color, line))
	l.base.Println(l.paint(color, fmt.Sprintf("%s %s", icon, l.compose(level, title, kv...))))
	l.base.Println(l.paint(color, line))
}

func (l *Logger) compose(level, message string, kv ...any) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("%s [%-5s] %s", timestamp, level, message)
	if len(kv) == 0 {
		return line
	}
	pairs := make([]string, 0, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		key := fmt.Sprintf("arg%d", i)
		value := "<missing>"
		key = fmt.Sprint(kv[i])
		if i+1 < len(kv) {
			value = formatValue(kv[i+1])
		}
		pairs = append(pairs, fmt.Sprintf("%s=%s", key, value))
	}
	return line + " | " + strings.Join(pairs, " ")
}

func (l *Logger) paint(color, message string) string {
	if !l.useColors {
		return message
	}
	return color + message + colorReset
}

func shouldUseColors() bool {
	term := strings.TrimSpace(os.Getenv("TERM"))
	if term == "" || term == "dumb" {
		return false
	}
	return true
}

func formatValue(value any) string {
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return `""`
		}
		if strings.ContainsAny(typed, " \t") {
			return fmt.Sprintf("%q", typed)
		}
		return typed
	case time.Time:
		return typed.Format(time.RFC3339)
	case time.Duration:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

func joinTableBorder(widths []int, sep string) string {
	parts := make([]string, 0, len(widths))
	for _, width := range widths {
		parts = append(parts, strings.Repeat("─", width+2))
	}
	return strings.Join(parts, sep)
}

func padRight(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func SortedKVLines(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s=%d", key, values[key]))
	}
	return lines
}
