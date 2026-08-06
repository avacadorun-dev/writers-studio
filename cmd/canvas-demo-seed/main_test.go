package main

import (
	"path/filepath"
	"testing"
)

func TestPublishedDemoArchiveCoversFeatureMatrix(t *testing.T) {
	path := filepath.Join("..", "..", "public", "docs", "examples", "echo-frontier-canvas-demo.wstudio")
	if err := validateProjectArchive(path); err != nil {
		t.Fatal(err)
	}
}

func TestRequireDemoValuesReportsMissingCapability(t *testing.T) {
	err := requireDemoValues("field types", map[string]bool{"markdown": true}, []string{"markdown", "canvas"})
	if err == nil || err.Error() != "demo archive field types do not contain canvas" {
		t.Fatalf("error = %v", err)
	}
}
