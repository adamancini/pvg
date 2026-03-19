package verify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScan_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result, err := Scan([]string{dir}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed {
		t.Error("expected pass for empty dir")
	}
	if result.FilesScanned != 0 {
		t.Errorf("expected 0 files scanned, got %d", result.FilesScanned)
	}
}

func TestScan_CleanFile(t *testing.T) {
	dir := t.TempDir()
	code := `package main

import "fmt"

func main() {
	fmt.Println("hello world")
	fmt.Println("this is a real program")
	fmt.Println("with enough substance")
	fmt.Println("to pass the thin file check")
	fmt.Println("because it has many lines")
	fmt.Println("of actual code")
	fmt.Println("and no stubs")
	fmt.Println("or todo markers")
	fmt.Println("or placeholder returns")
	fmt.Println("it is complete")
}
`
	writeTempFile(t, dir, "main.go", code)

	result, err := Scan([]string{dir}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed {
		t.Errorf("expected pass, got %d issues", len(result.Issues))
		for _, iss := range result.Issues {
			t.Logf("  %s:%d [%s] %s", iss.File, iss.Line, iss.Type, iss.Pattern)
		}
	}
}

func TestScan_DetectsNotImplementedError(t *testing.T) {
	dir := t.TempDir()
	code := `class Handler:
    def process(self, data):
        raise NotImplementedError("subclass must implement")
    def validate(self, data):
        if not data:
            raise ValueError("empty data")
        return True
    def transform(self, data):
        result = []
        for item in data:
            result.append(item.upper())
        return result
`
	writeTempFile(t, dir, "handler.py", code)

	result, err := Scan([]string{dir}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed {
		t.Error("expected failure for NotImplementedError")
	}
	found := false
	for _, iss := range result.Issues {
		if iss.Type == "stub" && iss.Line == 3 {
			found = true
		}
	}
	if !found {
		t.Error("expected stub issue on line 3")
	}
}

func TestScan_DetectsPanicTodo(t *testing.T) {
	dir := t.TempDir()
	code := `package auth

func ValidateToken(token string) (bool, error) {
	panic("not implemented")
}

func HashPassword(password string) string {
	return "hashed_" + password
}

func ComparePassword(hash, password string) bool {
	return hash == "hashed_"+password
}
`
	writeTempFile(t, dir, "auth.go", code)

	result, err := Scan([]string{dir}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed {
		t.Error("expected failure for panic(not implemented)")
	}
	assertHasIssue(t, result, "stub", 4)
}

func TestScan_DetectsReturnEmptyObject(t *testing.T) {
	dir := t.TempDir()
	code := `export function getUser(id: string): User {
  return {};
}

export function getConfig(): Config {
  return {
    host: "localhost",
    port: 3000,
    debug: false,
    maxRetries: 3,
    timeout: 5000,
  };
}
`
	writeTempFile(t, dir, "service.ts", code)

	result, err := Scan([]string{dir}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertHasIssue(t, result, "stub", 2)
}

func TestScan_DetectsTodoMarker(t *testing.T) {
	dir := t.TempDir()
	code := `package main

import "fmt"

func process() {
	// TODO: implement the actual processing logic
	fmt.Println("processing")
	fmt.Println("more processing")
	fmt.Println("even more processing")
	fmt.Println("final processing")
	fmt.Println("done")
}
`
	writeTempFile(t, dir, "process.go", code)

	result, err := Scan([]string{dir}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertHasIssue(t, result, "todo", 6)
}

func TestScan_DetectsBarePythonPass(t *testing.T) {
	dir := t.TempDir()
	code := `class Service:
    def connect(self):
        pass

    def disconnect(self):
        self.connection.close()
        self.connection = None
        self.connected = False
        self.retry_count = 0
        print("disconnected")
        self.log("disconnect complete")
`
	writeTempFile(t, dir, "service.py", code)

	result, err := Scan([]string{dir}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertHasIssue(t, result, "stub", 3)
}

func TestScan_DetectsThinFile(t *testing.T) {
	dir := t.TempDir()
	code := `package stub

func Placeholder() {}
`
	writeTempFile(t, dir, "stub.go", code)

	result, err := Scan([]string{dir}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, iss := range result.Issues {
		if iss.Type == "thin_file" {
			found = true
		}
	}
	if !found {
		t.Error("expected thin_file issue")
	}
}

func TestScan_SkipsTestFiles(t *testing.T) {
	dir := t.TempDir()
	code := `package auth

import "testing"

func TestSomething(t *testing.T) {
	// TODO: add more test cases
	panic("not implemented")
}
`
	writeTempFile(t, dir, "auth_test.go", code)

	result, err := Scan([]string{dir}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesScanned != 0 {
		t.Errorf("expected 0 files scanned (test file should be skipped), got %d", result.FilesScanned)
	}
}

func TestScan_ExplicitTestFileIsScanned(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "auth_test.go", `package auth

import "testing"

func TestSomething(t *testing.T) {
	panic("not implemented")
}
`)

	result, err := Scan([]string{path}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesScanned != 1 {
		t.Fatalf("expected explicit test file to be scanned, got %d files", result.FilesScanned)
	}
	assertHasIssue(t, result, "stub", 6)
}

func TestScan_IncludesTestFilesWhenOptedIn(t *testing.T) {
	dir := t.TempDir()
	code := `package auth

import "testing"

func TestSomething(t *testing.T) {
	// TODO: add more test cases
	panic("not implemented")
}

func TestOther(t *testing.T) {
	if true != true {
		t.Error("reality broken")
	}
}
`
	writeTempFile(t, dir, "auth_test.go", code)

	opts := DefaultOptions()
	opts.IncludeTests = true
	result, err := Scan([]string{dir}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesScanned != 1 {
		t.Errorf("expected 1 file scanned with IncludeTests, got %d", result.FilesScanned)
	}
}

func TestScan_SkipsNonSourceFiles(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "README.md", "# TODO: write docs\n")
	writeTempFile(t, dir, "config.yaml", "# TODO: configure\n")
	writeTempFile(t, dir, "data.json", `{"todo": true}`)

	result, err := Scan([]string{dir}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesScanned != 0 {
		t.Errorf("expected 0 source files scanned, got %d", result.FilesScanned)
	}
}

func TestScan_SkipsVendorAndNodeModules(t *testing.T) {
	dir := t.TempDir()
	vendorDir := filepath.Join(dir, "vendor", "lib")
	nodeDir := filepath.Join(dir, "node_modules", "pkg")
	_ = os.MkdirAll(vendorDir, 0755)
	_ = os.MkdirAll(nodeDir, 0755)

	writeTempFile(t, vendorDir, "lib.go", `package lib
func X() { panic("not implemented") }
`)
	writeTempFile(t, nodeDir, "index.js", `throw new Error("not implemented");`)

	result, err := Scan([]string{dir}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesScanned != 0 {
		t.Errorf("expected 0 files scanned (vendor/node_modules skipped), got %d", result.FilesScanned)
	}
}

func TestScan_SingleFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "app.py", `class App:
    def run(self):
        raise NotImplementedError("must override")
    def stop(self):
        print("stopping")
        self.cleanup()
        print("stopped")
        self.log("stop complete")
        self.notify_shutdown()
        self.save_state()
        return True
`)

	result, err := Scan([]string{path}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesScanned != 1 {
		t.Errorf("expected 1 file scanned, got %d", result.FilesScanned)
	}
	assertHasIssue(t, result, "stub", 3)
}

func TestScan_DetectsRustUnimplemented(t *testing.T) {
	dir := t.TempDir()
	code := `pub fn process(data: &[u8]) -> Vec<u8> {
    unimplemented!()
}

pub fn validate(data: &[u8]) -> bool {
    data.len() > 0 && data[0] != 0
}

pub fn transform(data: &[u8]) -> Vec<u8> {
    data.iter().map(|b| b.wrapping_add(1)).collect()
}
`
	writeTempFile(t, dir, "lib.rs", code)

	result, err := Scan([]string{dir}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertHasIssue(t, result, "stub", 2)
}

func TestScan_DetectsRustTodoMacro(t *testing.T) {
	dir := t.TempDir()
	code := `pub fn handler(req: Request) -> Response {
    todo!()
}

pub fn other_handler(req: Request) -> Response {
    Response::new(200, "OK".to_string())
}

pub fn third_handler(req: Request) -> Response {
    let body = format!("Hello, {}", req.path);
    Response::new(200, body)
}
`
	writeTempFile(t, dir, "handlers.rs", code)

	result, err := Scan([]string{dir}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertHasIssue(t, result, "stub", 2)
}

func TestScan_DetectsEllipsisBody(t *testing.T) {
	dir := t.TempDir()
	code := `class Protocol:
    def send(self, data: bytes) -> None:
        ...

    def receive(self) -> bytes:
        return self.buffer.read()

    def close(self) -> None:
        self.socket.shutdown()
        self.socket.close()
        self.connected = False
        self.log("closed")
`
	writeTempFile(t, dir, "protocol.py", code)

	result, err := Scan([]string{dir}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertHasIssue(t, result, "stub", 3)
}

func TestScan_DetectsJSThrowNotImplemented(t *testing.T) {
	dir := t.TempDir()
	code := `export class BaseHandler {
  handle(req) {
    throw new Error("not implemented");
  }

  validate(req) {
    if (!req.body) {
      throw new Error("missing body");
    }
    return true;
  }

  transform(data) {
    return data.map(item => item.toUpperCase());
  }
}
`
	writeTempFile(t, dir, "handler.js", code)

	result, err := Scan([]string{dir}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertHasIssue(t, result, "stub", 3)
	// Verify that throw new Error("missing body") is NOT flagged as a stub
	for _, iss := range result.Issues {
		if iss.Type == "stub" && iss.Line == 8 {
			t.Error("throw new Error('missing body') should not be flagged as stub")
		}
	}
}

func TestScan_CustomMinLines(t *testing.T) {
	dir := t.TempDir()
	code := `package small

func Add(a, b int) int {
	return a + b
}
`
	writeTempFile(t, dir, "small.go", code)

	// With default (10), should flag as thin
	result, err := Scan([]string{dir}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	hasThin := false
	for _, iss := range result.Issues {
		if iss.Type == "thin_file" {
			hasThin = true
		}
	}
	if !hasThin {
		t.Error("expected thin_file with default min_lines=10")
	}

	// With min_lines=3, should pass
	opts := DefaultOptions()
	opts.MinLines = 3
	result, err = Scan([]string{dir}, opts)
	if err != nil {
		t.Fatal(err)
	}
	hasThin = false
	for _, iss := range result.Issues {
		if iss.Type == "thin_file" {
			hasThin = true
		}
	}
	if hasThin {
		t.Error("should not flag thin_file with min_lines=3")
	}
}

func TestFormatText_Passed(t *testing.T) {
	r := &Result{Passed: true, FilesScanned: 5}
	text := FormatText(r)
	if text != "VERIFY: PASSED (5 files scanned, 0 issues)\n" {
		t.Errorf("unexpected output: %q", text)
	}
}

func TestFormatText_Failed(t *testing.T) {
	r := &Result{
		Passed:       false,
		FilesScanned: 3,
		Issues: []Issue{
			{File: "auth.go", Line: 4, Type: "stub", Pattern: "panic(not implemented)", Context: `panic("not implemented")`},
			{File: "main.go", Line: 10, Type: "todo", Pattern: "TODO/FIXME marker", Context: "// TODO: implement auth"},
		},
	}
	text := FormatText(r)
	if text == "" {
		t.Error("expected non-empty output")
	}
	if !contains(text, "VERIFY: FAILED") {
		t.Error("expected FAILED header")
	}
	if !contains(text, "Stubs: 1") {
		t.Error("expected stub count")
	}
	if !contains(text, "TODOs: 1") {
		t.Error("expected todo count")
	}
}

func TestFormatJSON(t *testing.T) {
	r := &Result{Passed: true, FilesScanned: 2}
	j, err := FormatJSON(r)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(j, `"passed": true`) {
		t.Error("expected passed: true in JSON")
	}
}

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"auth_test.go", true},
		{"auth.test.ts", true},
		{"auth.spec.js", true},
		{"test_auth.py", true},
		{"auth_test.py", true},
		{"tests/helper.py", true},
		{"__tests__/auth.ts", true},
		{"auth.go", false},
		{"auth.py", false},
		{"main.ts", false},
		{"testutils.go", false},
	}

	for _, tt := range tests {
		got := isTestFile(tt.path)
		if got != tt.expected {
			t.Errorf("isTestFile(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestIsCommentOnly(t *testing.T) {
	tests := []struct {
		line string
		ext  string
		want bool
	}{
		{"// a comment", ".go", true},
		{"/* block */", ".go", true},
		{"* continuation", ".go", true},
		{"code()", ".go", false},
		{"# python comment", ".py", true},
		{"code()", ".py", false},
	}

	for _, tt := range tests {
		got := isCommentOnly(tt.line, tt.ext)
		if got != tt.want {
			t.Errorf("isCommentOnly(%q, %q) = %v, want %v", tt.line, tt.ext, got, tt.want)
		}
	}
}

// E2e existence checks

func TestCheckE2e_NoE2eTests(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "main.go", "package main\nfunc main() {}\n")
	writeTempFile(t, dir, "main_test.go", "package main\nimport \"testing\"\nfunc TestMain(t *testing.T) {}\n")

	result, err := CheckE2e(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Found {
		t.Error("expected no e2e tests found")
	}
	if result.Count != 0 {
		t.Errorf("expected 0, got %d", result.Count)
	}
}

func TestCheckE2e_FindsE2eDir(t *testing.T) {
	dir := t.TempDir()
	e2eDir := filepath.Join(dir, "test", "e2e")
	_ = os.MkdirAll(e2eDir, 0755)
	writeTempFile(t, e2eDir, "login_test.go", "package e2e\nfunc TestLogin() {}\n")

	result, err := CheckE2e(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Found {
		t.Error("expected e2e tests found in e2e/ directory")
	}
	if result.Count != 1 {
		t.Errorf("expected 1, got %d", result.Count)
	}
}

func TestCheckE2e_FindsE2eFilename(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "auth_e2e_test.go", "package auth\nfunc TestE2e() {}\n")

	result, err := CheckE2e(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Found {
		t.Error("expected e2e tests found by filename pattern")
	}
}

func TestCheckE2e_FindsE2eSpecPattern(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "login.e2e.spec.ts", "describe('login', () => {})\n")

	result, err := CheckE2e(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Found {
		t.Error("expected e2e tests found by .e2e.spec pattern")
	}
}

func TestCheckE2e_SkipsVendor(t *testing.T) {
	dir := t.TempDir()
	vendorE2e := filepath.Join(dir, "vendor", "e2e")
	_ = os.MkdirAll(vendorE2e, 0755)
	writeTempFile(t, vendorE2e, "test.go", "package e2e\n")

	result, err := CheckE2e(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Found {
		t.Error("should not find e2e tests in vendor/")
	}
}

func TestFormatE2eText_Found(t *testing.T) {
	r := &E2eResult{Found: true, Count: 2, Files: []string{"test/e2e/a.go", "test/e2e/b.go"}}
	text := FormatE2eText(r)
	if !contains(text, "PASSED") {
		t.Error("expected PASSED")
	}
	if !contains(text, "2 e2e test files") {
		t.Error("expected count in output")
	}
}

func TestFormatE2eText_NotFound(t *testing.T) {
	r := &E2eResult{Found: false, Count: 0}
	text := FormatE2eText(r)
	if !contains(text, "FAILED") {
		t.Error("expected FAILED")
	}
}

func TestIsE2eFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"test/e2e/login_test.go", true},
		{"e2e/smoke_test.py", true},
		{"tests/end-to-end/flow.spec.ts", true},
		{"auth_e2e_test.go", true},
		{"login.e2e.spec.ts", true},
		{"test_e2e_auth.py", true},
		{"e2e_test.go", true},
		{"auth_test.go", false},
		{"main.go", false},
		{"auth.spec.ts", false},
	}

	for _, tt := range tests {
		got := isE2eFile(tt.path)
		if got != tt.expected {
			t.Errorf("isE2eFile(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

// Helpers

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertHasIssue(t *testing.T, result *Result, issueType string, line int) {
	t.Helper()
	for _, iss := range result.Issues {
		if iss.Type == issueType && iss.Line == line {
			return
		}
	}
	t.Errorf("expected %s issue on line %d, issues: %v", issueType, line, result.Issues)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
