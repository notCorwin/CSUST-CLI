package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/notCorwin/csust-cli/internal/adapter"
	"github.com/notCorwin/csust-cli/internal/contract"
)

const version = "0.4.0"

// Keep the compatibility adapters available when the Go binary is installed elsewhere.
//
//go:embed csust.py csust_cli/*.py csust_cli/features/*.py
var legacyFiles embed.FS

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if has(args, "--version") {
		fmt.Printf("csust %s\n", version)
		return 0
	}
	jsonMode := has(args, "--json") && !hasHelp(args)
	stdout, stderr, code, err := (adapter.Legacy{Files: legacyFiles}).Run(context.Background(), args, jsonMode)
	if len(stderr) > 0 {
		_, _ = os.Stderr.Write(stderr)
	}
	if err != nil {
		if jsonMode {
			writeError("adapter_error", err.Error())
		} else {
			fmt.Fprintf(os.Stderr, "错误: %s\n", err)
		}
		return 2
	}
	if jsonMode {
		if len(strings.TrimSpace(string(stdout))) == 0 {
			writeError("command_failed", "命令没有返回 JSON 结果")
			return codeOrTwo(code)
		}
		var normalized []byte
		var normalizeErr error
		if code == 0 {
			normalized, normalizeErr = contract.Normalize(stdout)
		} else {
			normalized, normalizeErr = contract.NormalizeError(stdout)
		}
		if normalizeErr != nil {
			writeError("contract_error", normalizeErr.Error())
			return 2
		}
		stdout = append(normalized, '\n')
	}
	_, _ = os.Stdout.Write(stdout)
	return code
}

func has(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

func hasHelp(args []string) bool {
	return has(args, "--help") || has(args, "-h")
}

func writeError(code, message string) {
	payload := map[string]any{
		"error":     message,
		"code":      code,
		"ok":        false,
		"submitted": false,
		"confirmed": false,
		"evidence":  "unknown",
	}
	encoded, _ := json.Marshal(payload)
	_, _ = os.Stdout.Write(append(encoded, '\n'))
}

func codeOrTwo(code int) int {
	if code == 0 {
		return 2
	}
	return code
}
