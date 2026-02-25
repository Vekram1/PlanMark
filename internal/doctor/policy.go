package doctor

import (
	"fmt"
	"strings"

	"github.com/vikramoddiraju/planmark/internal/diag"
	"github.com/vikramoddiraju/planmark/internal/ir"
)

func EvaluateHorizonReadiness(plan ir.PlanIR) []diag.Diagnostic {
	return EvaluateHorizonReadinessWithProfile(plan, "build")
}

func EvaluateHorizonReadinessWithProfile(plan ir.PlanIR, profile string) []diag.Diagnostic {
	normalized := strings.ToLower(strings.TrimSpace(profile))
	if normalized == "" {
		normalized = "loose"
	}
	diagnostics := make([]diag.Diagnostic, 0)

	for _, task := range plan.Semantic.Tasks {
		horizon := strings.ToLower(strings.TrimSpace(task.Horizon))
		switch horizon {
		case "":
			continue
		case "now":
			if !hasNonEmpty(task.Accept) {
				severity := diag.SeverityError
				if normalized == "loose" {
					severity = diag.SeverityWarning
				}
				diagnostics = append(diagnostics, diag.Diagnostic{
					Severity: severity,
					Code:     diag.CodeMissingAccept,
					Message:  fmt.Sprintf("horizon=now task %q requires at least one @accept", task.ID),
					Source:   diag.SourceSpan{Path: plan.PlanPath},
				})
			}
		case "next":
			if !hasNonEmpty(task.Accept) {
				diagnostics = append(diagnostics, diag.Diagnostic{
					Severity: diag.SeverityWarning,
					Code:     diag.CodeMissingAccept,
					Message:  fmt.Sprintf("horizon=next task %q should define @accept (recommended)", task.ID),
					Source:   diag.SourceSpan{Path: plan.PlanPath},
				})
			}
		case "later":
			// Intentionally permissive: later-horizon tasks can remain underspecified.
		default:
			diagnostics = append(diagnostics, diag.Diagnostic{
				Severity: diag.SeverityWarning,
				Code:     diag.CodeUnknownHorizon,
				Message:  fmt.Sprintf("task %q uses unknown horizon %q", task.ID, task.Horizon),
				Source:   diag.SourceSpan{Path: plan.PlanPath},
			})
		}
	}

	diag.Sort(diagnostics)
	return diagnostics
}

func hasNonEmpty(values []string) bool {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}
