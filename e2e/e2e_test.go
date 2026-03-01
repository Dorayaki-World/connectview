package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func projectRoot() string {
	dir, _ := os.Getwd()
	return filepath.Join(dir, "..")
}

func buildBinary(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "protoc-gen-connectview")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/protoc-gen-connectview")
	cmd.Dir = projectRoot()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\nOutput: %s", err, out)
	}
	return binaryPath
}

func testdataDir() string {
	return filepath.Join(projectRoot(), "testdata", "proto")
}

func TestE2E_Build(t *testing.T) {
	binaryPath := buildBinary(t)
	info, err := os.Stat(binaryPath)
	if err != nil {
		t.Fatalf("binary not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("binary is empty")
	}
}

func TestE2E_GenerateHTML(t *testing.T) {
	if _, err := exec.LookPath("protoc"); err != nil {
		t.Skip("protoc not found in PATH, skipping E2E test")
	}

	binaryPath := buildBinary(t)
	tmpDir := t.TempDir()

	protoDir := testdataDir()

	cmd := exec.Command("protoc",
		"--plugin=protoc-gen-connectview="+binaryPath,
		"--connectview_out="+tmpDir,
		"-I", protoDir,
		"greet/v1/greet.proto",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("protoc failed: %v\nOutput: %s", err, out)
	}

	outFile := filepath.Join(tmpDir, "index.html")
	content, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	html := string(content)

	checks := []struct {
		desc     string
		contains string
		negate   bool
	}{
		{"doctype", "<!DOCTYPE html>", false},
		{"service name", "GreetService", false},
		{"rpc name", "Greet", false},
		{"connect path", "/connectrpc.greet.v1.GreetService/Greet", false},
		{"request message", "GreetRequest", false},
		{"response message", "GreetResponse", false},
		{"field name", `"name"`, false},
		{"service comment", "GreetService provides greeting functionality.", false},
		{"embedded schema", "__CONNECTVIEW_SCHEMA__", false},
		{"no external CDN", "cdn.", true},
	}

	for _, c := range checks {
		found := strings.Contains(html, c.contains)
		if c.negate {
			if found {
				t.Errorf("HTML should NOT contain %s: %q", c.desc, c.contains)
			}
		} else {
			if !found {
				t.Errorf("HTML should contain %s: %q", c.desc, c.contains)
			}
		}
	}
}

func TestE2E_CrossFileImport(t *testing.T) {
	if _, err := exec.LookPath("protoc"); err != nil {
		t.Skip("protoc not found in PATH, skipping E2E test")
	}

	binaryPath := buildBinary(t)
	tmpDir := t.TempDir()

	protoDir := testdataDir()

	// order.proto imports user.proto — pass both as files to generate
	cmd := exec.Command("protoc",
		"--plugin=protoc-gen-connectview="+binaryPath,
		"--connectview_out="+tmpDir,
		"-I", protoDir,
		"order/v1/order.proto",
		"user/v1/user.proto",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("protoc failed: %v\nOutput: %s", err, out)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "index.html"))
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	html := string(content)

	checks := []string{
		"OrderService",      // service from order.proto
		"UserService",       // service from user.proto
		"CreateOrder",       // RPC
		"Order",             // message
		"OrderItem",         // nested message from order.proto
		"User",              // imported type from user.proto
		"Address",           // nested type in user.proto
	}
	for _, c := range checks {
		if !strings.Contains(html, c) {
			t.Errorf("HTML should contain %q", c)
		}
	}
}

func TestE2E_CrossFileImportOnlyOneFile(t *testing.T) {
	if _, err := exec.LookPath("protoc"); err != nil {
		t.Skip("protoc not found in PATH, skipping E2E test")
	}

	binaryPath := buildBinary(t)
	tmpDir := t.TempDir()

	protoDir := testdataDir()

	// Only pass order.proto — user.proto is an import dependency, not in FileToGenerate
	cmd := exec.Command("protoc",
		"--plugin=protoc-gen-connectview="+binaryPath,
		"--connectview_out="+tmpDir,
		"-I", protoDir,
		"order/v1/order.proto",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("protoc failed: %v\nOutput: %s", err, out)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "index.html"))
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	html := string(content)

	checks := []string{
		"OrderService",      // service from order.proto
		"CreateOrder",       // RPC
		"User",              // imported type from user.proto should be resolved
	}
	for _, c := range checks {
		if !strings.Contains(html, c) {
			t.Errorf("HTML should contain %q", c)
		}
	}

	// UserService should NOT appear — it's in user.proto which is only an import dep
	if strings.Contains(html, "UserService") {
		t.Error("HTML should NOT contain UserService (user.proto was not in FileToGenerate)")
	}
}

func TestE2E_GenerateMultipleProtos(t *testing.T) {
	if _, err := exec.LookPath("protoc"); err != nil {
		t.Skip("protoc not found in PATH, skipping E2E test")
	}

	binaryPath := buildBinary(t)
	tmpDir := t.TempDir()

	protoDir := testdataDir()

	cmd := exec.Command("protoc",
		"--plugin=protoc-gen-connectview="+binaryPath,
		"--connectview_out="+tmpDir,
		"-I", protoDir,
		"greet/v1/greet.proto",
		"user/v1/user.proto",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("protoc failed: %v\nOutput: %s", err, out)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "index.html"))
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	html := string(content)

	// Both services should be in the output
	for _, svc := range []string{"GreetService", "UserService"} {
		if !strings.Contains(html, svc) {
			t.Errorf("HTML should contain service %q", svc)
		}
	}
}
