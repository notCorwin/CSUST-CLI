package adapter

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
)

// Legacy runs the existing service adapters while the command entrypoint is Go.
// ponytail: one subprocess compatibility boundary; replace it with native Go adapters when Python support is retired.
type Legacy struct {
	WorkDir string
	Files   fs.FS
}

func (a Legacy) Run(ctx context.Context, args []string, jsonMode bool) ([]byte, []byte, int, error) {
	workDir := a.WorkDir
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return nil, nil, 2, err
		}
	}
	script, cleanup, err := a.prepareScript(workDir)
	if err != nil {
		return nil, nil, 2, err
	}
	defer cleanup()
	python, err := pythonPath(workDir)
	if err != nil {
		return nil, nil, 2, err
	}
	commandArgs := append([]string{script}, args...)
	if jsonMode && !hasJSON(args) {
		commandArgs = append(commandArgs, "--json")
	}
	command := exec.CommandContext(ctx, python, commandArgs...)
	command.Dir = workDir
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	if err == nil {
		return stdout.Bytes(), stderr.Bytes(), 0, nil
	}
	if exitError, ok := err.(*exec.ExitError); ok {
		return stdout.Bytes(), stderr.Bytes(), exitError.ExitCode(), nil
	}
	return stdout.Bytes(), stderr.Bytes(), 2, err
}

func (a Legacy) prepareScript(workDir string) (string, func(), error) {
	if value := os.Getenv("CSUST_LEGACY_SCRIPT"); value != "" {
		script, err := scriptPath(workDir)
		return script, func() {}, err
	}
	if a.Files == nil {
		script, err := scriptPath(workDir)
		return script, func() {}, err
	}
	root, err := os.MkdirTemp("", "csust-cli-legacy-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	err = fs.WalkDir(a.Files, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		content, readErr := fs.ReadFile(a.Files, path)
		if readErr != nil {
			return readErr
		}
		target := filepath.Join(root, filepath.FromSlash(path))
		if makeErr := os.MkdirAll(filepath.Dir(target), 0700); makeErr != nil {
			return makeErr
		}
		return os.WriteFile(target, content, 0600)
	})
	if err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("解包内置适配器失败: %w", err)
	}
	return filepath.Join(root, "csust.py"), cleanup, nil
}

func hasJSON(args []string) bool {
	for _, arg := range args {
		if arg == "--json" {
			return true
		}
	}
	return false
}

func pythonPath(workDir string) (string, error) {
	if value := os.Getenv("CSUST_PYTHON"); value != "" {
		if _, err := exec.LookPath(value); err == nil {
			return value, nil
		}
		return "", fmt.Errorf("CSUST_PYTHON 不可执行: %s", value)
	}
	for _, name := range []string{filepath.Join(workDir, ".venv", "bin", "python"), "python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("未找到 Python 运行时")
}

func scriptPath(workDir string) (string, error) {
	if value := os.Getenv("CSUST_LEGACY_SCRIPT"); value != "" {
		if info, err := os.Stat(value); err == nil && !info.IsDir() {
			return value, nil
		}
		return "", fmt.Errorf("CSUST_LEGACY_SCRIPT 不存在: %s", value)
	}
	candidates := []string{filepath.Join(workDir, "csust.py")}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), "csust.py"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("未找到 csust.py；请设置 CSUST_LEGACY_SCRIPT")
}
