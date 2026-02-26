package doctor

import (
	"errors"

	"github.com/vikramoddiraju/planmark/internal/compile"
	"github.com/vikramoddiraju/planmark/internal/diag"
)

func FormatDiagnosticsText(diagnostics []diag.Diagnostic) string {
	copyDiags := append([]diag.Diagnostic(nil), diagnostics...)
	diag.Sort(copyDiags)
	return diag.RenderText(copyDiags)
}

func FormatDiagnosticsRich(diagnostics []diag.Diagnostic) string {
	copyDiags := append([]diag.Diagnostic(nil), diagnostics...)
	diag.Sort(copyDiags)
	return diag.RenderRich(copyDiags)
}

func DiagnosticFromCompileError(err error) (diag.Diagnostic, bool) {
	var limitErr *compile.LimitError
	if !errors.As(err, &limitErr) {
		return diag.Diagnostic{}, false
	}
	source := diag.SourceSpan{Path: limitErr.Path}
	if limitErr.Line > 0 {
		source.StartLine = limitErr.Line
		source.EndLine = limitErr.Line
	}
	return diag.Diagnostic{
		Severity: diag.SeverityError,
		Code:     diag.CodeCompileLimitExceeded,
		Message:  limitErr.Error(),
		Source:   source,
	}, true
}
