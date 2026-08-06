package canvas

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestAnalyzeGraphFindsComponentsOrphansRootsLeavesAndCycles(t *testing.T) {
	raw := json.RawMessage(`{"nodes":[{"id":"a"},{"id":"b"},{"id":"c"},{"id":"orphan"}],"edges":[{"id":"ab","fromNode":"a","toNode":"b"},{"id":"bc","fromNode":"b","toNode":"c"},{"id":"cb","fromNode":"c","toNode":"b"}]}`)
	result, err := AnalyzeGraph(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Components) != 2 || len(result.Orphans) != 1 || result.Orphans[0] != "orphan" ||
		len(result.Cycles) != 1 || len(result.Cycles[0]) != 2 {
		t.Fatalf("analysis = %#v", result)
	}
	if len(result.Roots) != 2 || len(result.Leaves) != 1 {
		t.Fatalf("roots/leaves = %#v/%#v", result.Roots, result.Leaves)
	}
}

func TestAnalyzeGraphRejectsMalformedCanvasAndNormalizesCycleSignature(t *testing.T) {
	if _, err := AnalyzeGraph(json.RawMessage(`nope`)); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("malformed error = %v", err)
	}
	if cycleSignature([]string{"b", "c", "a"}) != cycleSignature([]string{"a", "b", "c"}) || cycleSignature(nil) != "" {
		t.Fatal("cycle signature is not rotation-stable")
	}
}
