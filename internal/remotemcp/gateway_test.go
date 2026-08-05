package remotemcp

import "testing"

func TestRemoteToolAnnotations(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"workspace.list_projects", "workspace.get_context", "change_set.list",
		"change_set.get", "change_set.review", "project.get_overview", "export.get",
	} {
		if annotation := remoteAnnotations(name); annotation == nil || !annotation.ReadOnlyHint {
			t.Fatalf("%s must be annotated read-only", name)
		}
	}
	for _, name := range []string{"transcript.retranscribe", "export.create"} {
		annotation := remoteAnnotations(name)
		if annotation == nil || annotation.ReadOnlyHint || !annotation.IdempotentHint {
			t.Fatalf("%s must advertise replay-safe action semantics: %+v", name, annotation)
		}
	}
	for _, name := range []string{"change_set.apply", "change_set.reject", "change_set.undo"} {
		annotation := remoteAnnotations(name)
		if annotation == nil || annotation.ReadOnlyHint || annotation.DestructiveHint == nil ||
			!*annotation.DestructiveHint || annotation.IdempotentHint {
			t.Fatalf("%s has unsafe annotations: %+v", name, annotation)
		}
	}
	for _, name := range []string{"tree.archive", "version.restore", "document.append"} {
		annotation := remoteAnnotations(name)
		if annotation == nil || annotation.ReadOnlyHint || annotation.DestructiveHint == nil ||
			*annotation.DestructiveHint {
			t.Fatalf("staged tool %s must be non-destructive before Apply: %+v", name, annotation)
		}
	}
}
