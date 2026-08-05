package httpapi

import (
	"testing"

	"writers.studio/internal/store"
)

func TestValidateProjectUpdate(t *testing.T) {
	validGoal := 80_000
	tests := []struct {
		name  string
		input updateProjectRequest
		want  string
	}{
		{
			name:  "valid metadata",
			input: updateProjectRequest{Title: "Новая глава", Language: "ru", Synopsis: "Замысел", WordGoal: &validGoal},
		},
		{
			name:  "empty title",
			input: updateProjectRequest{Language: "ru"},
			want:  "Название должно содержать от 1 до 120 символов",
		},
		{
			name:  "invalid language",
			input: updateProjectRequest{Title: "Project", Language: "de"},
			want:  "Выберите язык диктовки",
		},
		{
			name:  "invalid word goal",
			input: updateProjectRequest{Title: "Project", Language: "en", WordGoal: intPointer(0)},
			want:  "Цель должна быть от 1 до 10 000 000 слов",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validateProjectUpdate(test.input); got != test.want {
				t.Fatalf("validateProjectUpdate()=%q, want %q", got, test.want)
			}
		})
	}
}

func TestValidateProjectAssistantStepLimit(t *testing.T) {
	maximum := store.MaxProjectAssistantStepLimit
	for _, test := range []struct {
		name  string
		limit *int
		valid bool
	}{
		{name: "unlimited", valid: true},
		{name: "minimum", limit: intPointer(1), valid: true},
		{name: "maximum", limit: &maximum, valid: true},
		{name: "zero", limit: intPointer(0)},
		{name: "above maximum", limit: intPointer(maximum + 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := store.ValidProjectAssistantStepLimit(test.limit); got != test.valid {
				t.Fatalf("ValidProjectAssistantStepLimit()=%t, want %t", got, test.valid)
			}
		})
	}
}

func intPointer(value int) *int {
	return &value
}
