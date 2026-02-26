package diag

import (
	"fmt"
	"strings"
)

func RenderText(diagnostics []Diagnostic) string {
	lines := make([]string, 0, len(diagnostics))
	for _, d := range diagnostics {
		location := formatLocation(d)
		lines = append(lines, fmt.Sprintf("%s %s%s %s", d.Severity, d.Code, location, d.Message))
	}
	return strings.Join(lines, "\n")
}

func RenderRich(diagnostics []Diagnostic) string {
	errorCount := 0
	warningCount := 0
	for _, d := range diagnostics {
		switch d.Severity {
		case SeverityError:
			errorCount++
		case SeverityWarning:
			warningCount++
		}
	}

	lines := []string{
		"diagnostics:",
		fmt.Sprintf("  total: %d", len(diagnostics)),
		fmt.Sprintf("  errors: %d", errorCount),
		fmt.Sprintf("  warnings: %d", warningCount),
	}
	if len(diagnostics) == 0 {
		return strings.Join(lines, "\n")
	}

	lines = append(lines, "entries:")
	for idx, d := range diagnostics {
		lines = append(lines, fmt.Sprintf("  %d. [%s] %s", idx+1, strings.ToUpper(string(d.Severity)), d.Code))
		lines = append(lines, fmt.Sprintf("     message: %s", d.Message))
		if location := strings.TrimSpace(formatLocation(d)); location != "" {
			lines = append(lines, fmt.Sprintf("     source: %s", location))
		}
	}

	return strings.Join(lines, "\n")
}

func formatLocation(d Diagnostic) string {
	if d.Source.Path != "" && d.Source.StartLine > 0 {
		end := d.Source.EndLine
		if end < d.Source.StartLine {
			end = d.Source.StartLine
		}
		return fmt.Sprintf(" %s:%d-%d", d.Source.Path, d.Source.StartLine, end)
	}
	if d.Source.Path != "" {
		return " " + d.Source.Path
	}
	return ""
}
