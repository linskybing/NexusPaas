package imageregistry

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

// buildExecutionInput is everything an executor needs to run one image build.
// The context archive (if any) has already passed validateBuildContextArchive.
type buildExecutionInput struct {
	BuildID        string
	ImageReference string
	Dockerfile     string
	ContextArchive []byte
	BuildArgs      map[string]any
}

// buildExecutionResult reports the supply-chain pipeline outcome. Statuses on
// the build record are derived from it by the dispatcher; executors only state
// facts about what they produced.
type buildExecutionResult struct {
	ImageDigest  string
	SBOMDigest   string
	ScanPassed   bool
	ScanSummary  string
	SignatureRef string
	Logs         []string
}

type buildExecutor interface {
	Name() string
	Execute(ctx context.Context, in buildExecutionInput) (buildExecutionResult, error)
}

// newBuildExecutorFromConfig maps IMAGE_BUILD_EXECUTOR to an executor. Empty
// means dispatch is disabled and builds stay queued (safe default: no
// undeclared docker/cluster dependency at startup).
func newBuildExecutorFromConfig(cfg platform.Config) buildExecutor {
	switch cfg.ImageBuildExecutor {
	case "docker":
		return dockerBuildExecutor{}
	default:
		return nil
	}
}

// dockerBuildExecutor runs the full supply-chain pipeline with local tooling:
// docker build → docker push → syft SBOM → trivy scan (fail-closed) → cosign
// sign. It is the live-evidence executor; an in-cluster BuildKit Job executor
// is the tracked production follow-up (ADR 0008 / blocker-ledger §8 item 5).
type dockerBuildExecutor struct{}

func (dockerBuildExecutor) Name() string { return "docker" }

func (dockerBuildExecutor) Execute(ctx context.Context, in buildExecutionInput) (buildExecutionResult, error) {
	result := buildExecutionResult{}
	workdir, err := os.MkdirTemp("", "nexuspaas-build-"+in.BuildID+"-")
	if err != nil {
		return result, fmt.Errorf("create build workdir: %w", err)
	}
	defer os.RemoveAll(workdir)

	contextDir := filepath.Join(workdir, "context")
	if err := os.Mkdir(contextDir, 0o755); err != nil {
		return result, fmt.Errorf("create context dir: %w", err)
	}
	if len(in.ContextArchive) > 0 {
		if err := extractBuildContext(in.ContextArchive, contextDir); err != nil {
			return result, err
		}
	}
	if in.Dockerfile != "" {
		if err := os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte(in.Dockerfile), 0o644); err != nil {
			return result, fmt.Errorf("write dockerfile: %w", err)
		}
	}
	if _, err := os.Stat(filepath.Join(contextDir, "Dockerfile")); err != nil {
		return result, fmt.Errorf("build context has no Dockerfile")
	}

	run := func(step string, name string, args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = workdir
		out, err := cmd.CombinedOutput()
		text := strings.TrimSpace(string(out))
		if len(text) > 4000 {
			text = text[:4000] + "\n[truncated]"
		}
		result.Logs = append(result.Logs, fmt.Sprintf("[%s] $ %s %s\n%s", step, name, strings.Join(args, " "), text))
		if err != nil {
			return text, fmt.Errorf("%s failed: %w", step, err)
		}
		return text, nil
	}

	buildArgs := []string{"build", "-t", in.ImageReference}
	for key, value := range in.BuildArgs {
		buildArgs = append(buildArgs, "--build-arg", fmt.Sprintf("%s=%v", key, value))
	}
	buildArgs = append(buildArgs, contextDir)
	if _, err := run("build", "docker", buildArgs...); err != nil {
		return result, err
	}
	if _, err := run("push", "docker", "push", in.ImageReference); err != nil {
		return result, err
	}
	digestOut, err := run("digest", "docker", "inspect", "--format", "{{index .RepoDigests 0}}", in.ImageReference)
	if err != nil {
		return result, err
	}
	if at := strings.LastIndex(digestOut, "@"); at >= 0 {
		result.ImageDigest = strings.TrimSpace(digestOut[at+1:])
	}
	if result.ImageDigest == "" {
		return result, fmt.Errorf("pushed image has no repo digest")
	}

	sbomPath := filepath.Join(workdir, "sbom.spdx.json")
	if _, err := run("sbom", "syft", "docker:"+in.ImageReference, "-o", "spdx-json="+sbomPath); err != nil {
		return result, err
	}
	sbomBytes, err := os.ReadFile(sbomPath)
	if err != nil {
		return result, fmt.Errorf("read sbom: %w", err)
	}
	result.SBOMDigest = imageBuildHash(string(sbomBytes))

	scanOut, scanErr := run("scan", "trivy", "image", "--quiet", "--scanners", "vuln", "--severity", "HIGH,CRITICAL", "--exit-code", "1", in.ImageReference)
	result.ScanSummary = lastLine(scanOut)
	if scanErr != nil {
		// trivy exit 1 = findings; the dispatcher records the scan failure and
		// fails the build without treating it as an infrastructure error.
		result.ScanPassed = false
		return result, nil
	}
	result.ScanPassed = true

	signRef := in.ImageReference
	if at := strings.LastIndex(signRef, ":"); at > strings.LastIndex(signRef, "/") {
		signRef = signRef[:at]
	}
	signRef = signRef + "@" + result.ImageDigest
	keyPath, cleanupKey, err := cosignKeyPath(ctx, workdir, &result)
	if err != nil {
		return result, err
	}
	defer cleanupKey()
	signCmd := exec.CommandContext(ctx, "cosign", "sign", "--yes", "--tlog-upload=false", "--key", keyPath, signRef)
	signCmd.Dir = workdir
	signCmd.Env = append(os.Environ(), "COSIGN_PASSWORD="+os.Getenv("COSIGN_PASSWORD"))
	signOut, err := signCmd.CombinedOutput()
	result.Logs = append(result.Logs, "[sign] $ cosign sign "+signRef+"\n"+strings.TrimSpace(string(signOut)))
	if err != nil {
		return result, fmt.Errorf("sign failed: %w", err)
	}
	result.SignatureRef = signRef
	return result, nil
}

// cosignKeyPath returns the signing key: COSIGN_KEY when configured, otherwise
// an ephemeral keypair generated into the build workdir (live-evidence runs).
func cosignKeyPath(ctx context.Context, workdir string, result *buildExecutionResult) (string, func(), error) {
	if configured := strings.TrimSpace(os.Getenv("COSIGN_KEY")); configured != "" {
		return configured, func() {}, nil
	}
	cmd := exec.CommandContext(ctx, "cosign", "generate-key-pair")
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(), "COSIGN_PASSWORD=")
	out, err := cmd.CombinedOutput()
	result.Logs = append(result.Logs, "[keygen] $ cosign generate-key-pair\n"+strings.TrimSpace(string(out)))
	if err != nil {
		return "", func() {}, fmt.Errorf("cosign keygen failed: %w", err)
	}
	return filepath.Join(workdir, "cosign.key"), func() {}, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[len(lines)-1])
}

// extractBuildContext unpacks a validated archive into dir. Paths are
// re-normalized (defense in depth) and only regular files are written, exactly
// matching what validateBuildContextArchive accepted.
func extractBuildContext(archive []byte, dir string) error {
	if bytes.HasPrefix(archive, []byte{0x1f, 0x8b}) {
		return extractTarGzBuildContext(archive, dir)
	}
	return extractZipBuildContext(archive, dir)
}

func extractTarGzBuildContext(archive []byte, dir string) error {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("open context archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read context archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if err := writeBuildContextFile(dir, hdr.Name, io.LimitReader(tr, maxBuildContextUncompressedBytes)); err != nil {
			return err
		}
	}
}

func extractZipBuildContext(archive []byte, dir string) error {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return fmt.Errorf("open context archive: %w", err)
	}
	for _, file := range zr.File {
		if file.FileInfo().IsDir() {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("open context entry %q: %w", file.Name, err)
		}
		err = writeBuildContextFile(dir, file.Name, io.LimitReader(rc, maxBuildContextUncompressedBytes))
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func writeBuildContextFile(dir, name string, body io.Reader) error {
	clean, err := normalizeBuildContextPath(name)
	if err != nil {
		return err
	}
	target := filepath.Join(dir, filepath.FromSlash(clean))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create context subdir: %w", err)
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create context file: %w", err)
	}
	_, err = io.Copy(out, body)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write context file: %w", err)
	}
	return nil
}
