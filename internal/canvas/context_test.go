package canvas

import "testing"

func TestValidateSelection(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"nodes":[{"id":"node-a","type":"text","x":0,"y":0,"width":100,"height":100,"text":"A"}],"edges":[]}`)
	if err := ValidateSelection(raw, []string{"node-a"}); err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][]string{{"missing"}, {"node-a", "node-a"}} {
		if ValidateSelection(raw, ids) == nil {
			t.Fatalf("invalid selection was accepted: %#v", ids)
		}
	}
	tooMany := make([]string, 501)
	if ValidateSelection(raw, tooMany) == nil {
		t.Fatal("oversized selection was accepted")
	}
}
