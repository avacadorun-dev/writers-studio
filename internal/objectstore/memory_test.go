package objectstore

import (
	"context"
	"testing"
)

func TestMemoryStoreCopiesInputAndOutput(t *testing.T) {
	t.Parallel()

	store := NewMemory()
	input := []byte("audio")
	if err := store.Put(context.Background(), "chunk", "audio/webm", input); err != nil {
		t.Fatal(err)
	}
	input[0] = 'X'
	output, err := store.Get(context.Background(), "chunk")
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "audio" {
		t.Fatalf("stored object mutated: %q", output)
	}
	output[0] = 'Y'
	again, _ := store.Get(context.Background(), "chunk")
	if string(again) != "audio" {
		t.Fatalf("returned object was not copied: %q", again)
	}
}
