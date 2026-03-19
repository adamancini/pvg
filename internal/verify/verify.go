// Package verify provides deterministic quality checks for delivered code.
// It scans source files for stub patterns, thin files, and TODO markers
// that indicate incomplete implementation. Designed to run as a pre-review
// gate before LLM-based verification (PM-Acceptor).
package verify

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Issue represents a single verification finding.
type Issue struct {
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	Type    string `json:"type"`    // "stub", "todo", "thin_file"
	Pattern string `json:"pattern"` // what was matched
	Context string `json:"context"` // the line content (trimmed)
}

// Result is the output of a verification run.
type Result struct {
	Passed       bool    `json:"passed"`
	FilesScanned int     `json:"files_scanned"`
	Issues       []Issue `json:"issues"`
}

// Options configures the verification scan.
type Options struct {
	MinLines     int  // minimum lines of code for substance check (default 10)
	IncludeTests bool // include test files in scan (default false)
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() Options {
	return Options{
		MinLines:     10,
		IncludeTests: false,
	}
}

// stubPattern defines a regex pattern and its metadata.
type stubPattern struct {
	re       *regexp.Regexp
	category string // "stub" or "todo"
	desc     string
}

// Patterns are ordered by specificity (most specific first).
var patterns = []stubPattern{
	// Not-implemented errors (unambiguous stubs)
	{re: regexp.MustCompile(`(?i)raise\s+NotImplementedError`), category: "stub", desc: "raise NotImplementedError"},
	{re: regexp.MustCompile(`(?i)throw\s+new\s+Error\(\s*["'](not\s*implemented|todo|fixme)["']`), category: "stub", desc: "throw Error(not implemented)"},
	{re: regexp.MustCompile(`(?i)panic\(\s*["'](not\s*implemented|todo)["']\s*\)`), category: "stub", desc: "panic(not implemented)"},
	{re: regexp.MustCompile(`unimplemented!\(\)`), category: "stub", desc: "unimplemented!()"},
	{re: regexp.MustCompile(`todo!\(\)`), category: "stub", desc: "todo!()"},

	// Return empty values as sole implementation
	{re: regexp.MustCompile(`^\s*return\s+\{\s*\}\s*;?\s*$`), category: "stub", desc: "return {}"},
	{re: regexp.MustCompile(`^\s*return\s+\[\s*\]\s*;?\s*$`), category: "stub", desc: "return []"},
	{re: regexp.MustCompile(`^\s*return\s+["']["']\s*;?\s*$`), category: "stub", desc: "return empty string"},

	// Python bare pass (stub indicator in function/method body)
	{re: regexp.MustCompile(`^\s*pass\s*(#.*)?$`), category: "stub", desc: "bare pass statement"},

	// Python ellipsis as function body
	{re: regexp.MustCompile(`^\s*\.\.\.\s*$`), category: "stub", desc: "ellipsis body (...)"},

	// TODO/FIXME markers (informational -- worth knowing about)
	{re: regexp.MustCompile(`(?i)\b(TODO|FIXME|XXX|HACK)\s*[:\-]`), category: "todo", desc: "TODO/FIXME marker"},
}

// Source file extensions to scan.
var sourceExtensions = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".py": true, ".rs": true, ".rb": true, ".java": true, ".cs": true,
	".swift": true, ".kt": true, ".ex": true, ".exs": true,
	".c": true, ".cpp": true, ".h": true, ".hpp": true,
}

// Test file patterns to exclude.
var testFilePatterns = []*regexp.Regexp{
	regexp.MustCompile(`_test\.go$`),
	regexp.MustCompile(`\.test\.[jt]sx?$`),
	regexp.MustCompile(`\.spec\.[jt]sx?$`),
	regexp.MustCompile(`(?i)^test_.*\.py$`),
	regexp.MustCompile(`(?i).*_test\.py$`),
	regexp.MustCompile(`(?i)Test\.java$`),
	regexp.MustCompile(`(?i)Tests?\.cs$`),
}

// Directories to always skip.
var skipDirs = map[string]bool{
	".git": true, ".svn": true, "node_modules": true, "vendor": true,
	"__pycache__": true, ".mypy_cache": true, ".pytest_cache": true,
	"dist": true, "build": true, ".next": true, "target": true,
	".vault": true, ".claude": true,
}

// Scan runs verification on the given paths (files or directories).
// If paths is empty, scans the current directory.
func Scan(paths []string, opts Options) (*Result, error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	if opts.MinLines <= 0 {
		opts.MinLines = 10
	}

	result := &Result{Passed: true}

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("cannot access %s: %w", p, err)
		}

		if info.IsDir() {
			if err := scanDir(p, opts, result); err != nil {
				return nil, err
			}
		} else {
			scanFile(p, opts, result, true)
		}
	}

	result.Passed = len(result.Issues) == 0
	return result, nil
}

// FormatText returns a human-readable report.
func FormatText(r *Result) string {
	var sb strings.Builder
	if r.Passed {
		fmt.Fprintf(&sb, "VERIFY: PASSED (%d files scanned, 0 issues)\n", r.FilesScanned)
		return sb.String()
	}

	stubs := 0
	todos := 0
	thins := 0
	for _, iss := range r.Issues {
		switch iss.Type {
		case "stub":
			stubs++
		case "todo":
			todos++
		case "thin_file":
			thins++
		}
	}

	fmt.Fprintf(&sb, "VERIFY: FAILED (%d files scanned, %d issues)\n", r.FilesScanned, len(r.Issues))
	if stubs > 0 {
		fmt.Fprintf(&sb, "  Stubs: %d (incomplete implementation)\n", stubs)
	}
	if todos > 0 {
		fmt.Fprintf(&sb, "  TODOs: %d (unfinished markers)\n", todos)
	}
	if thins > 0 {
		fmt.Fprintf(&sb, "  Thin files: %d (below substance threshold)\n", thins)
	}
	sb.WriteString("\n")

	for _, iss := range r.Issues {
		if iss.Line > 0 {
			fmt.Fprintf(&sb, "  %s:%d [%s] %s\n", iss.File, iss.Line, iss.Type, iss.Pattern)
			if iss.Context != "" {
				fmt.Fprintf(&sb, "    > %s\n", iss.Context)
			}
		} else {
			fmt.Fprintf(&sb, "  %s [%s] %s\n", iss.File, iss.Type, iss.Pattern)
		}
	}

	return sb.String()
}

// FormatJSON returns a JSON report.
func FormatJSON(r *Result) (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// scanDir walks a directory and scans source files.
func scanDir(dir string, opts Options, result *Result) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible files
		}

		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		scanFile(path, opts, result, false)
		return nil
	})
}

// scanFile checks a single file for stub patterns and substance.
func scanFile(path string, opts Options, result *Result, explicitFile bool) {
	ext := filepath.Ext(path)
	if !sourceExtensions[ext] {
		return
	}

	// Skip test files unless opted in
	if !opts.IncludeTests && !explicitFile && isTestFile(path) {
		return
	}

	result.FilesScanned++

	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	codeLines := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Count lines of actual code (non-blank, non-comment-only)
		if trimmed != "" && !isCommentOnly(trimmed, ext) {
			codeLines++
		}

		// Check against patterns
		for _, p := range patterns {
			if p.re.MatchString(line) {
				result.Issues = append(result.Issues, Issue{
					File:    path,
					Line:    lineNum,
					Type:    p.category,
					Pattern: p.desc,
					Context: truncate(trimmed, 120),
				})
				break // one match per line is enough
			}
		}
	}

	// Thin file check
	if codeLines > 0 && codeLines < opts.MinLines {
		result.Issues = append(result.Issues, Issue{
			File:    path,
			Type:    "thin_file",
			Pattern: fmt.Sprintf("%d lines of code (threshold: %d)", codeLines, opts.MinLines),
		})
	}
}

// isTestFile checks if a file is a test file based on name patterns.
func isTestFile(path string) bool {
	base := filepath.Base(path)

	// Check directory-based test locations
	dir := filepath.Dir(path)
	for _, part := range strings.Split(dir, string(filepath.Separator)) {
		lower := strings.ToLower(part)
		if lower == "test" || lower == "tests" || lower == "__tests__" ||
			lower == "testdata" || lower == "fixtures" {
			return true
		}
	}

	// Check filename patterns
	for _, pat := range testFilePatterns {
		if pat.MatchString(base) {
			return true
		}
	}
	return false
}

// isCommentOnly checks if a line is only a comment (language-aware).
func isCommentOnly(trimmed, ext string) bool {
	switch ext {
	case ".py", ".rb", ".ex", ".exs":
		return strings.HasPrefix(trimmed, "#")
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".java", ".cs", ".rs",
		".c", ".cpp", ".h", ".hpp", ".swift", ".kt":
		return strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*")
	}
	return false
}

// E2eResult is the output of an e2e existence check.
type E2eResult struct {
	Found bool     `json:"found"`
	Count int      `json:"count"`
	Files []string `json:"files"`
}

// e2e directory names (case-insensitive match).
var e2eDirs = []string{"e2e", "end-to-end", "end_to_end"}

// e2eFilePattern matches filenames containing "e2e" alongside a test indicator.
var e2eFilePattern = regexp.MustCompile(`(?i)(e2e.*test|test.*e2e|e2e.*spec|spec.*e2e|_e2e_|\.e2e\.)`)

// CheckE2e scans the given root directory for e2e test files.
// It looks for: (1) files in directories named e2e/ (any depth),
// (2) source files whose name matches common e2e test patterns.
func CheckE2e(root string) (*E2eResult, error) {
	if root == "" {
		root = "."
	}

	result := &E2eResult{}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		if !sourceExtensions[filepath.Ext(path)] {
			return nil
		}

		if isE2eFile(path) {
			result.Files = append(result.Files, path)
			result.Count++
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("scan for e2e tests: %w", err)
	}

	result.Found = result.Count > 0
	return result, nil
}

// FormatE2eText returns a human-readable report for the e2e check.
func FormatE2eText(r *E2eResult) string {
	if r.Found {
		s := fmt.Sprintf("E2E CHECK: PASSED (%d e2e test files found)\n", r.Count)
		for _, f := range r.Files {
			s += fmt.Sprintf("  %s\n", f)
		}
		return s
	}
	return "E2E CHECK: FAILED (0 e2e test files found)\n" +
		"  No files found in e2e/ directories or matching e2e test naming patterns.\n" +
		"  Every epic requires e2e tests exercising the full system from the user's perspective.\n"
}

// isE2eFile checks if a file is an e2e test based on directory or filename.
func isE2eFile(path string) bool {
	// Check if any directory component is an e2e directory
	for _, part := range strings.Split(filepath.Dir(path), string(filepath.Separator)) {
		lower := strings.ToLower(part)
		for _, e2eDir := range e2eDirs {
			if lower == e2eDir {
				return true
			}
		}
	}

	// Check filename pattern
	return e2eFilePattern.MatchString(filepath.Base(path))
}

// truncate shortens a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
