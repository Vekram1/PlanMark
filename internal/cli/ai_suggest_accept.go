package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vikramoddiraju/planmark/internal/ai"
	"github.com/vikramoddiraju/planmark/internal/compile"
	"github.com/vikramoddiraju/planmark/internal/config"
	contextpkg "github.com/vikramoddiraju/planmark/internal/context"
	"github.com/vikramoddiraju/planmark/internal/diag"
	"github.com/vikramoddiraju/planmark/internal/doctor"
	"github.com/vikramoddiraju/planmark/internal/protocol"
)

type suggestAcceptResult struct {
	TaskID      string   `json:"task_id"`
	PlanPath    string   `json:"plan_path"`
	Horizon     string   `json:"horizon"`
	Runnable    bool     `json:"runnable"`
	Suggestions []string `json:"suggestions"`
}

func runAI(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: plan ai <subcommand> [args]")
		fmt.Fprintln(stderr, "subcommands: suggest-accept, summarize-closure, draft-beads, suggest-fix, apply-fix")
		return protocol.ExitUsageError
	}

	switch args[0] {
	case "suggest-accept":
		return runAISuggestAccept(args[1:], stdout, stderr)
	case "summarize-closure":
		return runAISummarizeClosure(args[1:], stdout, stderr)
	case "draft-beads":
		return runAIDraftBeads(args[1:], stdout, stderr)
	case "suggest-fix":
		return runAISuggestFix(args[1:], stdout, stderr)
	case "apply-fix":
		return runAIApplyFix(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown ai command: %s\n", args[0])
		return protocol.ExitUsageError
	}
}

func runAISuggestAccept(args []string, stdout io.Writer, stderr io.Writer) int {
	positionalID := ""
	filteredArgs := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--plan=") || strings.HasPrefix(arg, "--format=") {
			filteredArgs = append(filteredArgs, arg)
			continue
		}
		if arg == "--plan" || arg == "--format" {
			filteredArgs = append(filteredArgs, arg)
			if i+1 < len(args) {
				i++
				filteredArgs = append(filteredArgs, args[i])
			}
			continue
		}
		if strings.HasPrefix(arg, "-") {
			filteredArgs = append(filteredArgs, arg)
			continue
		}
		if positionalID == "" {
			positionalID = arg
			continue
		}
		filteredArgs = append(filteredArgs, arg)
	}

	fs := flag.NewFlagSet("ai suggest-accept", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "", "path to `plan-file` markdown file")
	format := fs.String("format", "text", "output format: text|json")
	if err := fs.Parse(filteredArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return protocol.ExitSuccess
		}
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}

	remaining := fs.Args()
	if len(remaining) > 1 {
		fmt.Fprintln(stderr, "usage: plan ai suggest-accept <id> --plan <path> [--format text|json]")
		return protocol.ExitUsageError
	}
	if len(remaining) == 1 {
		if positionalID != "" {
			fmt.Fprintln(stderr, "too many positional arguments for ai suggest-accept")
			return protocol.ExitUsageError
		}
		positionalID = remaining[0]
	}
	if strings.TrimSpace(positionalID) == "" {
		fmt.Fprintln(stderr, "usage: plan ai suggest-accept <id> --plan <path> [--format text|json]")
		return protocol.ExitUsageError
	}
	if strings.TrimSpace(*planPath) == "" {
		fmt.Fprintln(stderr, "missing --plan")
		return protocol.ExitUsageError
	}

	content, err := os.ReadFile(*planPath)
	if err != nil {
		fmt.Fprintf(stderr, "read plan: %v\n", err)
		return protocol.ExitInternalError
	}
	compiled, err := compile.CompilePlan(*planPath, content, compile.NewParser(nil))
	if err != nil {
		fmt.Fprintf(stderr, "compile plan: %v\n", err)
		return protocol.ExitInternalError
	}

	taskID := strings.TrimSpace(positionalID)
	explain, err := contextpkg.Explain(compiled, taskID)
	if err != nil {
		if errors.Is(err, contextpkg.ErrTaskNotFound) {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitValidationFailed
		}
		fmt.Fprintf(stderr, "explain task: %v\n", err)
		return protocol.ExitInternalError
	}

	suggestions := suggestAcceptLines(explain.Blockers)
	if len(suggestions) == 0 {
		suggestions = []string{
			"@accept cmd:<command>",
		}
	}

	result := suggestAcceptResult{
		TaskID:      explain.TaskID,
		PlanPath:    compiled.PlanPath,
		Horizon:     explain.Horizon,
		Runnable:    explain.Runnable,
		Suggestions: suggestions,
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		status := "ok"
		if len(result.Suggestions) == 0 {
			status = "validation_failed"
		}
		payload := protocol.Envelope[suggestAcceptResult]{
			SchemaVersion: protocol.SchemaVersionV01,
			ToolVersion:   CLIVersion,
			Command:       "ai suggest-accept",
			Status:        status,
			Data:          result,
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitInternalError
		}
	case "text":
		fmt.Fprintf(stdout, "task_id: %s\n", result.TaskID)
		fmt.Fprintf(stdout, "plan_path: %s\n", result.PlanPath)
		fmt.Fprintf(stdout, "horizon: %s\n", result.Horizon)
		fmt.Fprintf(stdout, "runnable: %t\n", result.Runnable)
		fmt.Fprintln(stdout, "suggestions:")
		for _, suggestion := range result.Suggestions {
			fmt.Fprintf(stdout, "- %s\n", suggestion)
		}
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}

	return protocol.ExitSuccess
}

func suggestAcceptLines(blockers []contextpkg.ExplainBlocker) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	for _, blocker := range blockers {
		for _, suggestion := range blockerSuggestions(blocker.Code) {
			if _, ok := seen[suggestion]; ok {
				continue
			}
			seen[suggestion] = struct{}{}
			out = append(out, suggestion)
		}
	}
	sort.Strings(out)
	return out
}

func blockerSuggestions(code string) []string {
	switch strings.TrimSpace(code) {
	case "MISSING_ACCEPT":
		return []string{
			"@accept cmd:<command>",
			"@accept file:<path> exists",
		}
	case "UNKNOWN_DEPENDENCY", "MISSING_DEP":
		return []string{
			"@accept cmd:plan doctor --plan <path> --profile exec",
		}
	default:
		return nil
	}
}

type suggestFixItem struct {
	Code         string `json:"code"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	SourcePath   string `json:"source_path,omitempty"`
	StartLine    int    `json:"start_line,omitempty"`
	EndLine      int    `json:"end_line,omitempty"`
	SuggestedFix string `json:"suggested_fix"`
}

type suggestFixResult struct {
	PlanPath        string           `json:"plan_path"`
	Profile         string           `json:"profile"`
	DiagnosticCount int              `json:"diagnostic_count"`
	Prompt          string           `json:"prompt"`
	Repairs         []suggestFixItem `json:"repairs,omitempty"`
}

type applyFixProposal struct {
	ProposalType string           `json:"proposal_type"`
	BasePlanHash string           `json:"base_plan_hash"`
	Provider     string           `json:"provider,omitempty"`
	Model        string           `json:"model,omitempty"`
	ProviderText string           `json:"provider_text,omitempty"`
	Repairs      []suggestFixItem `json:"repairs,omitempty"`
}

type applyFixResult struct {
	PlanPath            string           `json:"plan_path"`
	Profile             string           `json:"profile"`
	Approved            bool             `json:"approved"`
	PlanMutated         bool             `json:"plan_mutated"`
	ProviderConfigured  bool             `json:"provider_configured"`
	Prompt              string           `json:"prompt"`
	Proposal            applyFixProposal `json:"proposal"`
	PreDiagnosticCount  int              `json:"pre_diagnostic_count"`
	PostDiagnosticCount int              `json:"post_diagnostic_count"`
}

func runAISuggestFix(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("ai suggest-fix", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "", "path to `plan-file` markdown file")
	format := fs.String("format", "text", "output format: text|json")
	profile := fs.String("profile", "build", "doctor profile: loose|build|exec")
	limit := fs.Int("limit", 20, "max repair suggestions to emit")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return protocol.ExitSuccess
		}
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}
	if strings.TrimSpace(*planPath) == "" {
		fmt.Fprintln(stderr, "missing --plan")
		return protocol.ExitUsageError
	}
	if *limit <= 0 {
		fmt.Fprintln(stderr, "--limit must be > 0")
		return protocol.ExitUsageError
	}

	normalizedProfile, err := doctor.NormalizeProfile(*profile)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}

	content, err := os.ReadFile(*planPath)
	if err != nil {
		fmt.Fprintf(stderr, "read plan: %v\n", err)
		return protocol.ExitInternalError
	}

	compiled, err := compile.CompilePlan(*planPath, content, compile.NewParser(nil))
	diagnostics := make([]diag.Diagnostic, 0)
	if err != nil {
		if d, ok := doctor.DiagnosticFromCompileError(err); ok {
			diagnostics = append(diagnostics, d)
		} else {
			fmt.Fprintf(stderr, "compile plan: %v\n", err)
			return protocol.ExitInternalError
		}
	} else {
		doctorResult, err := doctor.Run(compiled, normalizedProfile)
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitUsageError
		}
		diagnostics = append(diagnostics, doctorResult.Diagnostics...)
	}

	repairs := buildSuggestFixRepairs(diagnostics, *limit)
	result := suggestFixResult{
		PlanPath:        filepathToSlash(*planPath),
		Profile:         normalizedProfile,
		DiagnosticCount: len(diagnostics),
		Prompt:          buildSuggestFixPrompt(filepathToSlash(*planPath), normalizedProfile, repairs),
		Repairs:         repairs,
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		payload := protocol.Envelope[suggestFixResult]{
			SchemaVersion: protocol.SchemaVersionV01,
			ToolVersion:   CLIVersion,
			Command:       "ai suggest-fix",
			Status:        "ok",
			Data:          result,
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitInternalError
		}
	case "text":
		fmt.Fprintf(stdout, "plan_path: %s\n", result.PlanPath)
		fmt.Fprintf(stdout, "profile: %s\n", result.Profile)
		fmt.Fprintf(stdout, "diagnostic_count: %d\n", result.DiagnosticCount)
		fmt.Fprintf(stdout, "repair_count: %d\n", len(result.Repairs))
		fmt.Fprintln(stdout, "prompt:")
		fmt.Fprintln(stdout, result.Prompt)
		if len(result.Repairs) > 0 {
			fmt.Fprintln(stdout, "repairs:")
			for _, repair := range result.Repairs {
				fmt.Fprintf(stdout, "- [%s] %s\n", repair.Code, repair.Message)
				if repair.SourcePath != "" {
					fmt.Fprintf(stdout, "  source: %s:%d-%d\n", repair.SourcePath, repair.StartLine, repair.EndLine)
				}
				fmt.Fprintf(stdout, "  suggested_fix: %s\n", repair.SuggestedFix)
			}
		}
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}

	return protocol.ExitSuccess
}

func buildSuggestFixRepairs(diagnostics []diag.Diagnostic, limit int) []suggestFixItem {
	repairs := make([]suggestFixItem, 0, minInt(limit, len(diagnostics)))
	for _, d := range diagnostics {
		if len(repairs) >= limit {
			break
		}
		repairs = append(repairs, suggestFixItem{
			Code:         string(d.Code),
			Severity:     string(d.Severity),
			Message:      strings.TrimSpace(d.Message),
			SourcePath:   strings.TrimSpace(d.Source.Path),
			StartLine:    d.Source.StartLine,
			EndLine:      d.Source.EndLine,
			SuggestedFix: suggestFixForCode(string(d.Code)),
		})
	}
	return repairs
}

func buildSuggestFixPrompt(planPath string, profile string, repairs []suggestFixItem) string {
	lines := []string{
		"You are editing PLAN.md using deterministic constraints.",
		"Do not invent tasks that are not implied by diagnostics.",
		"Return either a minimal markdown patch or a Plan Delta proposal.",
		"Reference diagnostic codes exactly when describing each fix.",
		"Target file: " + strings.TrimSpace(planPath),
		"Profile: " + strings.TrimSpace(profile),
	}
	if len(repairs) == 0 {
		lines = append(lines, "No diagnostics found. No repair patch required.")
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "Diagnostics to address:")
	for idx, repair := range repairs {
		source := repair.SourcePath
		if source == "" {
			source = "<unknown>"
		}
		start := strconv.Itoa(repair.StartLine)
		end := strconv.Itoa(repair.EndLine)
		if repair.StartLine <= 0 {
			start = "?"
		}
		if repair.EndLine <= 0 {
			end = "?"
		}
		lines = append(lines, fmt.Sprintf("%d. [%s] %s (%s:%s-%s)", idx+1, repair.Code, repair.Message, source, start, end))
		lines = append(lines, "   Suggested direction: "+repair.SuggestedFix)
	}
	return strings.Join(lines, "\n")
}

func suggestFixForCode(code string) string {
	switch strings.TrimSpace(code) {
	case string(diag.CodeMissingAccept):
		return "Add one or more @accept lines for the task, using explicit command/file checks."
	case string(diag.CodeUnknownDependency):
		return "Fix or remove unknown @deps references so each dependency resolves to an existing task id."
	case string(diag.CodeDependencyCycle):
		return "Break dependency cycles by removing or restructuring @deps edges."
	case string(diag.CodeDuplicateTaskID):
		return "Make task @id values unique while preserving intended dependency links."
	case string(diag.CodeUnknownHorizon):
		return "Normalize @horizon to one of now|next|later."
	case string(diag.CodeUnattachedMetadata):
		return "Attach metadata directly under a target task/section or remove stray metadata lines."
	case string(diag.CodePathTraversalReject):
		return "Rewrite path metadata to remain within repository scope."
	case string(diag.CodeCompileLimitExceeded):
		return "Split oversized content/lines into smaller sections to satisfy compile limits."
	default:
		return "Apply the minimal deterministic PLAN.md edit needed to resolve this diagnostic."
	}
}

func filepathToSlash(path string) string {
	return strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
}

func runAIApplyFix(args []string, stdout io.Writer, stderr io.Writer) int {
	timeoutFlagExplicit := hasIntFlagArg(args, "--timeout-seconds")

	fs := flag.NewFlagSet("ai apply-fix", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "", "path to `plan-file` markdown file")
	format := fs.String("format", "text", "output format: text|json")
	profile := fs.String("profile", "build", "doctor profile: loose|build|exec")
	limit := fs.Int("limit", 20, "max repair suggestions to emit")
	providerFlag := fs.String("provider", "", "ai provider override")
	modelFlag := fs.String("model", "", "ai model override")
	baseURLFlag := fs.String("base-url", "", "provider base URL override")
	apiKeyEnvFlag := fs.String("api-key-env", "", "provider API key env var override")
	timeoutSecFlag := fs.Int("timeout-seconds", 30, "provider request timeout in seconds")
	approve := fs.Bool("approve", false, "explicit approval to produce apply-fix proposal")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return protocol.ExitSuccess
		}
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}

	if !*approve {
		fmt.Fprintln(stderr, "explicit approval required: re-run with --approve")
		return protocol.ExitValidationFailed
	}
	if strings.TrimSpace(*planPath) == "" {
		fmt.Fprintln(stderr, "missing --plan")
		return protocol.ExitUsageError
	}
	if *limit <= 0 {
		fmt.Fprintln(stderr, "--limit must be > 0")
		return protocol.ExitUsageError
	}
	if *timeoutSecFlag <= 0 {
		fmt.Fprintln(stderr, "--timeout-seconds must be > 0")
		return protocol.ExitUsageError
	}

	normalizedProfile, err := doctor.NormalizeProfile(*profile)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}

	raw, err := os.ReadFile(*planPath)
	if err != nil {
		fmt.Fprintf(stderr, "read plan: %v\n", err)
		return protocol.ExitInternalError
	}
	basePlanHash := sha256HexBytes(raw)

	preDiags, err := collectDoctorDiagnostics(*planPath, raw, normalizedProfile)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitInternalError
	}
	repairs := buildSuggestFixRepairs(preDiags, *limit)
	prompt := buildSuggestFixPrompt(filepathToSlash(*planPath), normalizedProfile, repairs)

	cfgResolved, cfgErr := config.LoadForPlan(*planPath)
	if cfgErr != nil {
		fmt.Fprintf(stderr, "load config: %v\n", cfgErr)
		return protocol.ExitUsageError
	}

	effectiveProvider := strings.TrimSpace(*providerFlag)
	if effectiveProvider == "" {
		effectiveProvider = strings.TrimSpace(cfgResolved.AI.Provider)
	}
	effectiveModel := strings.TrimSpace(*modelFlag)
	if effectiveModel == "" {
		effectiveModel = strings.TrimSpace(cfgResolved.AI.Model)
	}
	effectiveBaseURL := strings.TrimSpace(*baseURLFlag)
	if effectiveBaseURL == "" {
		effectiveBaseURL = strings.TrimSpace(cfgResolved.AI.BaseURL)
	}
	effectiveAPIKeyEnv := strings.TrimSpace(*apiKeyEnvFlag)
	if effectiveAPIKeyEnv == "" {
		effectiveAPIKeyEnv = strings.TrimSpace(cfgResolved.AI.APIKeyEnv)
	}
	effectiveTimeout := time.Duration(*timeoutSecFlag) * time.Second
	if timeoutFromConfig, err := strconv.Atoi(strings.TrimSpace(cfgResolved.AI.TimeoutSec)); err == nil && timeoutFromConfig > 0 && !timeoutFlagExplicit {
		effectiveTimeout = time.Duration(timeoutFromConfig) * time.Second
	}

	// Re-run compile+doctor checks as an explicit post-check before success.
	postDiags, err := collectDoctorDiagnostics(*planPath, raw, normalizedProfile)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitInternalError
	}

	providerConfigured := effectiveProvider != ""
	proposal := applyFixProposal{
		ProposalType: "plan_delta_preview",
		BasePlanHash: basePlanHash,
		Repairs:      repairs,
	}
	if providerConfigured {
		var provider ai.Provider
		switch effectiveProvider {
		case ai.ProviderDeterministic:
			provider, err = ai.NewProvider(effectiveProvider)
			if err != nil {
				fmt.Fprintf(stderr, "provider init: %v\n", err)
				return protocol.ExitUsageError
			}
		case ai.ProviderOpenAICompatible:
			provider, err = ai.NewOpenAICompatibleProvider(ai.OpenAICompatibleConfig{
				BaseURL: effectiveBaseURL,
				APIKey:  strings.TrimSpace(os.Getenv(effectiveAPIKeyEnv)),
				Model:   effectiveModel,
				Timeout: effectiveTimeout,
			})
			if err != nil {
				fmt.Fprintf(stderr, "provider init: %v\n", err)
				return protocol.ExitUsageError
			}
		default:
			fmt.Fprintf(stderr, "unsupported provider: %s\n", effectiveProvider)
			return protocol.ExitUsageError
		}

		providerResp, err := provider.GenerateApplyFix(context.Background(), ai.ApplyFixRequest{
			PlanPath: filepathToSlash(*planPath),
			PlanHash: basePlanHash,
			Prompt:   prompt,
			Model:    effectiveModel,
		})
		if err != nil {
			fmt.Fprintf(stderr, "provider generate: %v\n", err)
			return protocol.ExitInternalError
		}
		proposal.Provider = providerResp.Provider
		proposal.Model = providerResp.Model
		proposal.ProviderText = providerResp.ProposalText
		if strings.TrimSpace(providerResp.ProposalType) != "" {
			proposal.ProposalType = strings.TrimSpace(providerResp.ProposalType)
		}
	}

	result := applyFixResult{
		PlanPath: filepathToSlash(*planPath),
		Profile:  normalizedProfile,
		Approved: true,
		// apply-fix currently emits a reviewable proposal and does not mutate PLAN.md in-place.
		PlanMutated:         false,
		ProviderConfigured:  providerConfigured,
		Prompt:              prompt,
		Proposal:            proposal,
		PreDiagnosticCount:  len(preDiags),
		PostDiagnosticCount: len(postDiags),
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		payload := protocol.Envelope[applyFixResult]{
			SchemaVersion: protocol.SchemaVersionV01,
			ToolVersion:   CLIVersion,
			Command:       "ai apply-fix",
			Status:        "ok",
			Data:          result,
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitInternalError
		}
	case "text":
		fmt.Fprintf(stdout, "plan_path: %s\n", result.PlanPath)
		fmt.Fprintf(stdout, "profile: %s\n", result.Profile)
		fmt.Fprintf(stdout, "approved: %t\n", result.Approved)
		fmt.Fprintf(stdout, "plan_mutated: %t\n", result.PlanMutated)
		fmt.Fprintf(stdout, "provider_configured: %t\n", result.ProviderConfigured)
		fmt.Fprintf(stdout, "proposal_type: %s\n", result.Proposal.ProposalType)
		fmt.Fprintf(stdout, "base_plan_hash: %s\n", result.Proposal.BasePlanHash)
		if result.Proposal.Provider != "" {
			fmt.Fprintf(stdout, "provider: %s\n", result.Proposal.Provider)
		}
		if result.Proposal.Model != "" {
			fmt.Fprintf(stdout, "provider_model: %s\n", result.Proposal.Model)
		}
		fmt.Fprintf(stdout, "pre_diagnostic_count: %d\n", result.PreDiagnosticCount)
		fmt.Fprintf(stdout, "post_diagnostic_count: %d\n", result.PostDiagnosticCount)
		fmt.Fprintln(stdout, "prompt:")
		fmt.Fprintln(stdout, result.Prompt)
		if len(result.Proposal.Repairs) > 0 {
			fmt.Fprintln(stdout, "proposal_repairs:")
			for _, repair := range result.Proposal.Repairs {
				fmt.Fprintf(stdout, "- [%s] %s\n", repair.Code, repair.Message)
				fmt.Fprintf(stdout, "  suggested_fix: %s\n", repair.SuggestedFix)
			}
		}
		if strings.TrimSpace(result.Proposal.ProviderText) != "" {
			fmt.Fprintln(stdout, "provider_output:")
			fmt.Fprintln(stdout, result.Proposal.ProviderText)
		}
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", *format)
		return protocol.ExitUsageError
	}

	return protocol.ExitSuccess
}

func collectDoctorDiagnostics(planPath string, raw []byte, profile string) ([]diag.Diagnostic, error) {
	compiled, err := compile.CompilePlan(planPath, raw, compile.NewParser(nil))
	if err != nil {
		if d, ok := doctor.DiagnosticFromCompileError(err); ok {
			return []diag.Diagnostic{d}, nil
		}
		return nil, fmt.Errorf("compile plan: %w", err)
	}
	result, err := doctor.Run(compiled, profile)
	if err != nil {
		return nil, fmt.Errorf("doctor run: %w", err)
	}
	return result.Diagnostics, nil
}

func sha256HexBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func hasIntFlagArg(args []string, flagName string) bool {
	for _, arg := range args {
		if arg == flagName || strings.HasPrefix(arg, flagName+"=") {
			return true
		}
	}
	return false
}
