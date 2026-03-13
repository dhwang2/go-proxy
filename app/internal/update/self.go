package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/dhwang2/go-proxy/pkg/fileutil"
	gh "github.com/dhwang2/go-proxy/pkg/github"
)

type Options struct {
	Repo       string
	Branch     string
	Token      string
	WorkDir    string
	BinaryPath string
	DryRun     bool
}

type Result struct {
	CurrentRef  string
	RemoteRef   string
	NeedsUpdate bool
	Updated     bool
	Message     string
	BackupPath  string
}

func SelfUpdate() error {
	_, err := Run(context.Background(), Options{})
	return err
}

func Run(ctx context.Context, opts Options) (Result, error) {
	if strings.TrimSpace(opts.Repo) == "" {
		opts.Repo = "dhwang2/go-proxy"
	}
	if strings.TrimSpace(opts.Branch) == "" {
		opts.Branch = "main"
	}
	if strings.TrimSpace(opts.WorkDir) == "" {
		opts.WorkDir = "/etc/go-proxy"
	}
	if strings.TrimSpace(opts.BinaryPath) == "" {
		path, err := os.Executable()
		if err != nil {
			return Result{}, err
		}
		if resolved, err := filepath.EvalSymlinks(path); err == nil && strings.TrimSpace(resolved) != "" {
			path = resolved
		}
		opts.BinaryPath = path
	}
	if strings.TrimSpace(opts.Token) == "" {
		opts.Token = readToken(opts.WorkDir)
	}

	client := gh.NewClient()
	client.Token = strings.TrimSpace(opts.Token)

	return runRepoMode(ctx, client, opts)
}

func runRepoMode(ctx context.Context, client *gh.Client, opts Options) (Result, error) {
	run, err := client.LatestSuccessfulWorkflowRun(ctx, opts.Repo, GoProxyCIWorkflowFile, opts.Branch)
	if err != nil {
		return Result{}, err
	}
	sha := strings.TrimSpace(run.HeadSHA)
	if sha == "" {
		return Result{}, errors.New("workflow run returned empty head sha")
	}
	artifacts, err := client.ListWorkflowArtifacts(ctx, opts.Repo, run.ID)
	if err != nil {
		return Result{}, err
	}
	artifact, err := selectRepoArtifact(artifacts)
	if err != nil {
		return Result{}, err
	}
	localRef := readLocalRef(filepath.Join(opts.WorkDir, ".script_source_ref"))
	result := Result{
		CurrentRef:  localRef,
		RemoteRef:   sha,
		NeedsUpdate: localRef == "" || !strings.HasPrefix(sha, localRef),
	}
	if !result.NeedsUpdate {
		result.Message = "already up to date"
		return result, nil
	}
	if opts.DryRun {
		result.Message = "dry-run: repo commit drift detected"
		return result, nil
	}

	workspace, err := os.MkdirTemp("", "proxy-update-repo-*")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(workspace)

	archivePath := filepath.Join(workspace, "artifact.zip")
	if err := DownloadArtifactArchive(ctx, artifact.DownloadURL, opts.Token, archivePath); err != nil {
		return result, err
	}
	compiled := filepath.Join(workspace, "proxy-new")
	if err := extractRepoArtifactBinary(archivePath, compiled); err != nil {
		return result, err
	}

	backup, err := replaceBinary(opts.BinaryPath, compiled)
	if err != nil {
		return result, err
	}
	_ = os.WriteFile(filepath.Join(opts.WorkDir, ".script_source_ref"), []byte(sha+"\n"), 0o644)
	result.Updated = true
	result.BackupPath = backup
	result.Message = "binary updated from repo artifact"
	return result, nil
}

func artifactBinaryCandidates() []string {
	out := []string{"proxy-linux-" + runtime.GOARCH}
	switch runtime.GOARCH {
	case "amd64":
		out = append(out, "proxy")
	case "arm64":
		out = append(out, "proxy-arm64")
	}
	return out
}

func extractRepoArtifactBinary(archivePath, outPath string) error {
	var lastErr error
	for _, name := range artifactBinaryCandidates() {
		if err := ExtractZipBinary(archivePath, name, outPath); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no matching repo binary candidate found")
	}
	return lastErr
}

func selectRepoArtifact(artifacts []gh.WorkflowArtifact) (gh.WorkflowArtifact, error) {
	target := "go-proxy-linux-" + runtime.GOARCH
	for _, artifact := range artifacts {
		if artifact.Expired {
			continue
		}
		if strings.TrimSpace(artifact.Name) == target {
			return artifact, nil
		}
	}
	return gh.WorkflowArtifact{}, fmt.Errorf("no repo artifact matched runtime %s/%s", runtime.GOOS, runtime.GOARCH)
}

func replaceBinary(targetPath, srcBinary string) (string, error) {
	st, err := os.Stat(srcBinary)
	if err != nil {
		return "", err
	}
	if st.Mode()&0o111 == 0 {
		if err := os.Chmod(srcBinary, 0o755); err != nil {
			return "", err
		}
	}
	backup, err := fileutil.BackupFile(targetPath, filepath.Dir(targetPath))
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(srcBinary)
	if err != nil {
		return backup, err
	}
	if err := fileutil.AtomicWrite(targetPath, content, 0o755); err != nil {
		return backup, err
	}
	return backup, nil
}

func readToken(workDir string) string {
	patPath := filepath.Join(workDir, ".pat")
	b, err := os.ReadFile(patPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readLocalRef(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(string(b))
	if v == "" {
		return ""
	}
	re := regexp.MustCompile(`[0-9a-fA-F]{7,40}`)
	if m := re.FindString(v); m != "" {
		return strings.ToLower(m)
	}
	return strings.ToLower(v)
}
