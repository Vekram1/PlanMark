package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/vikramoddiraju/planmark/internal/protocol"
)

const (
	defaultRepoSlug      = "Vekram1/PlanMark"
	defaultGitHubAPIBase = "https://api.github.com"
	defaultRawRepoBranch = "master"
)

type releaseMetadata struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

type updateData struct {
	CurrentVersion  string `json:"current_version"`
	CurrentTag      string `json:"current_tag"`
	TargetRef       string `json:"target_ref"`
	LatestTag       string `json:"latest_tag,omitempty"`
	LatestVersion   string `json:"latest_version,omitempty"`
	ReleaseURL      string `json:"release_url,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	Updated         bool   `json:"updated"`
	InstallDir      string `json:"install_dir,omitempty"`
	Message         string `json:"message,omitempty"`
}

type installRequest struct {
	InstallDir string
	TargetRef  string
	RepoSlug   string
}

var (
	updateHTTPClient = &http.Client{Timeout: 15 * time.Second}
	executablePath   = os.Executable
	runUpdateInstall = defaultRunUpdateInstall
)

func runUpdate(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(stderr)
	checkOnly := fs.Bool("check", false, "check for a newer release without installing it")
	format := fs.String("format", "text", "output format: text|json")
	channel := fs.String("channel", "stable", "release channel: stable|edge")
	ref := fs.String("ref", "", "install exact `release-tag-or-ref` instead of resolving the channel default")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return protocol.ExitSuccess
		}
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitUsageError
	}

	selectedChannel := strings.TrimSpace(*channel)
	switch selectedChannel {
	case "stable", "edge":
	default:
		fmt.Fprintf(stderr, "invalid --channel value: %s\n", selectedChannel)
		return protocol.ExitUsageError
	}

	exePath, err := executablePath()
	if err != nil {
		fmt.Fprintf(stderr, "resolve executable path: %v\n", err)
		return protocol.ExitInternalError
	}
	installDir := filepath.Dir(exePath)
	repoSlug := repoSlug()
	currentTag := "v" + CLIVersion

	result, err := resolveUpdateData(context.Background(), selectedChannel, strings.TrimSpace(*ref), repoSlug, installDir)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return protocol.ExitInternalError
	}
	result.CurrentVersion = CLIVersion
	result.CurrentTag = currentTag
	result.InstallDir = installDir

	if *checkOnly {
		return writeUpdateOutput(*format, result, stdout, stderr)
	}

	if !result.UpdateAvailable && strings.TrimSpace(*ref) == "" {
		result.Message = fmt.Sprintf("planmark is already up to date at %s", result.CurrentTag)
		return writeUpdateOutput(*format, result, stdout, stderr)
	}

	if runtime.GOOS == "windows" {
		result.Message = windowsManualUpdateMessage()
		return writeUpdateOutput(*format, result, stdout, stderr)
	}

	if err := runUpdateInstall(installRequest{
		InstallDir: installDir,
		TargetRef:  result.TargetRef,
		RepoSlug:   repoSlug,
	}); err != nil {
		fmt.Fprintf(stderr, "run installer: %v\n", err)
		return protocol.ExitInternalError
	}

	result.Updated = true
	result.Message = fmt.Sprintf("updated planmark in %s to %s", installDir, result.TargetRef)
	return writeUpdateOutput(*format, result, stdout, stderr)
}

func resolveUpdateData(ctx context.Context, channel string, ref string, repoSlug string, installDir string) (updateData, error) {
	result := updateData{InstallDir: installDir}
	if trimmedRef := strings.TrimSpace(ref); trimmedRef != "" {
		result.TargetRef = trimmedRef
		result.UpdateAvailable = trimmedRef != "v"+CLIVersion
		return result, nil
	}
	if channel == "edge" {
		result.TargetRef = defaultRawRepoBranch
		result.UpdateAvailable = true
		result.Message = "edge updates install the current default branch build"
		return result, nil
	}

	release, err := fetchLatestRelease(ctx, repoSlug)
	if err != nil {
		return updateData{}, err
	}
	result.TargetRef = release.TagName
	result.LatestTag = release.TagName
	result.LatestVersion = strings.TrimPrefix(release.TagName, "v")
	result.ReleaseURL = release.HTMLURL
	result.UpdateAvailable = release.TagName != "v"+CLIVersion
	return result, nil
}

func fetchLatestRelease(ctx context.Context, repoSlug string) (releaseMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL(repoSlug), nil)
	if err != nil {
		return releaseMetadata{}, fmt.Errorf("build latest release request: %w", err)
	}
	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return releaseMetadata{}, fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return releaseMetadata{}, fmt.Errorf("fetch latest release: unexpected status %s", resp.Status)
	}
	var release releaseMetadata
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return releaseMetadata{}, fmt.Errorf("decode latest release: %w", err)
	}
	if strings.TrimSpace(release.TagName) == "" {
		return releaseMetadata{}, errors.New("latest release response is missing tag_name")
	}
	return release, nil
}

func latestReleaseURL(repoSlug string) string {
	return strings.TrimRight(githubAPIBaseURL(), "/") + "/repos/" + repoSlug + "/releases/latest"
}

func githubAPIBaseURL() string {
	if override := strings.TrimSpace(os.Getenv("PLANMARK_GITHUB_API_BASE_URL")); override != "" {
		return override
	}
	return defaultGitHubAPIBase
}

func rawInstallURL(repoSlug string, scriptName string) string {
	if override := strings.TrimSpace(os.Getenv("PLANMARK_RAW_BASE_URL")); override != "" {
		return strings.TrimRight(override, "/") + "/" + scriptName
	}
	return "https://raw.githubusercontent.com/" + repoSlug + "/" + defaultRawRepoBranch + "/scripts/" + scriptName
}

func repoSlug() string {
	if override := strings.TrimSpace(os.Getenv("PLANMARK_REPO")); override != "" {
		return override
	}
	return defaultRepoSlug
}

func writeUpdateOutput(format string, data updateData, stdout io.Writer, stderr io.Writer) int {
	payload := protocol.Envelope[updateData]{
		SchemaVersion: protocol.SchemaVersionV01,
		ToolVersion:   CLIVersion,
		Command:       "update",
		Status:        "ok",
		Data:          data,
	}
	switch format {
	case "json":
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return protocol.ExitInternalError
		}
		return protocol.ExitSuccess
	case "text":
		fmt.Fprintf(stdout, "current_tag: %s\n", data.CurrentTag)
		fmt.Fprintf(stdout, "target_ref: %s\n", data.TargetRef)
		if data.LatestTag != "" {
			fmt.Fprintf(stdout, "latest_tag: %s\n", data.LatestTag)
		}
		fmt.Fprintf(stdout, "update_available: %t\n", data.UpdateAvailable)
		if data.InstallDir != "" {
			fmt.Fprintf(stdout, "install_dir: %s\n", data.InstallDir)
		}
		if data.ReleaseURL != "" {
			fmt.Fprintf(stdout, "release_url: %s\n", data.ReleaseURL)
		}
		if data.Message != "" {
			fmt.Fprintf(stdout, "message: %s\n", data.Message)
		}
		return protocol.ExitSuccess
	default:
		fmt.Fprintf(stderr, "invalid --format value: %s\n", format)
		return protocol.ExitUsageError
	}
}

func defaultRunUpdateInstall(req installRequest) error {
	scriptName := "install.sh"
	shellName := "bash"
	if runtime.GOOS == "windows" {
		scriptName = "install.ps1"
		shellName = "powershell"
	}
	if _, err := exec.LookPath(shellName); err != nil {
		return fmt.Errorf("missing required shell %q", shellName)
	}

	tmpDir, err := os.MkdirTemp("", "planmark-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	scriptPath := filepath.Join(tmpDir, scriptName)
	if err := downloadFile(scriptPath, rawInstallURL(req.RepoSlug, scriptName)); err != nil {
		return err
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	default:
		cmd = exec.Command("bash", scriptPath)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"PLANMARK_INSTALL_DIR="+req.InstallDir,
		"PLANMARK_REPO="+req.RepoSlug,
		"PLANMARK_REF="+req.TargetRef,
	)
	return cmd.Run()
}

func downloadFile(dst string, src string) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, src, nil)
	if err != nil {
		return err
	}
	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download installer: unexpected status %s", resp.Status)
	}
	file, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := io.Copy(file, resp.Body); err != nil {
		return err
	}
	return nil
}

func windowsManualUpdateMessage() string {
	return `Windows update currently requires rerunning the installer: powershell -ExecutionPolicy Bypass -c "irm https://raw.githubusercontent.com/Vekram1/PlanMark/master/scripts/install.ps1 | iex"`
}
