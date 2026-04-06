package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateHelpReturnsZeroAndShowsFlags(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	exit := Run([]string{"update", "--help"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0 for update help, got %d", exit)
	}
	rendered := errOut.String()
	if !strings.Contains(rendered, "check for a newer release without installing it") {
		t.Fatalf("expected update help to describe --check, got %q", rendered)
	}
	if !strings.Contains(rendered, "install exact release-tag-or-ref instead of resolving the channel default") {
		t.Fatalf("expected update help to describe --ref, got %q", rendered)
	}
}

func TestUpdateCheckJSONIncludesLatestRelease(t *testing.T) {
	newerTag := fmt.Sprintf("v%s-next", CLIVersion)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/Vekram1/PlanMark/releases/latest" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"` + newerTag + `","html_url":"https://example.test/releases/` + newerTag + `"}`))
	}))
	defer server.Close()

	t.Setenv("PLANMARK_GITHUB_API_BASE_URL", server.URL)

	fakeExe := filepath.Join(t.TempDir(), "bin", "planmark")
	restoreExe := executablePath
	executablePath = func() (string, error) { return fakeExe, nil }
	t.Cleanup(func() { executablePath = restoreExe })

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"update", "--check", "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}

	var payload struct {
		Data updateData `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal update json: %v", err)
	}
	if payload.Data.CurrentTag != "v"+CLIVersion {
		t.Fatalf("expected current tag v%s, got %+v", CLIVersion, payload.Data)
	}
	if payload.Data.LatestTag != newerTag {
		t.Fatalf("expected latest tag %s, got %+v", newerTag, payload.Data)
	}
	if !payload.Data.UpdateAvailable {
		t.Fatalf("expected update_available=true, got %+v", payload.Data)
	}
}

func TestUpdateUsesInstallerForLatestStableRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.1.6","html_url":"https://example.test/releases/v0.1.6"}`))
	}))
	defer server.Close()

	t.Setenv("PLANMARK_GITHUB_API_BASE_URL", server.URL)

	fakeExe := filepath.Join(t.TempDir(), "bin", "planmark")
	restoreExe := executablePath
	executablePath = func() (string, error) { return fakeExe, nil }
	t.Cleanup(func() { executablePath = restoreExe })

	restoreInstall := runUpdateInstall
	var captured installRequest
	runUpdateInstall = func(req installRequest) error {
		captured = req
		return nil
	}
	t.Cleanup(func() { runUpdateInstall = restoreInstall })

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"update", "--format", "json"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	if captured.TargetRef != "v0.1.6" {
		t.Fatalf("expected installer target v0.1.6, got %+v", captured)
	}
	if captured.InstallDir != filepath.Dir(fakeExe) {
		t.Fatalf("expected installer dir %s, got %+v", filepath.Dir(fakeExe), captured)
	}
}

func TestUpdateCheckReportsAlreadyCurrentRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v` + CLIVersion + `","html_url":"https://example.test/releases/v` + CLIVersion + `"}`))
	}))
	defer server.Close()

	t.Setenv("PLANMARK_GITHUB_API_BASE_URL", server.URL)

	fakeExe := filepath.Join(t.TempDir(), "bin", "planmark")
	restoreExe := executablePath
	executablePath = func() (string, error) { return fakeExe, nil }
	t.Cleanup(func() { executablePath = restoreExe })

	var out bytes.Buffer
	var errOut bytes.Buffer
	exit := Run([]string{"update", "--check", "--format", "text"}, &out, &errOut)
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", exit, errOut.String())
	}
	rendered := out.String()
	if !strings.Contains(rendered, "update_available: false") {
		t.Fatalf("expected update check to report false, got %q", rendered)
	}
	if !strings.Contains(rendered, "latest_tag: v"+CLIVersion) {
		t.Fatalf("expected latest tag in output, got %q", rendered)
	}
}
