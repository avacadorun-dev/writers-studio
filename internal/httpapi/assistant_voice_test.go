package httpapi

import "testing"

func TestParseAssistantVoiceCommandRequiresExactReservedPhrase(t *testing.T) {
	tests := []struct {
		text string
		want assistantVoiceCommand
	}{
		{"Команда, выполнить поручение!", assistantVoiceCommandExecute},
		{"  команда — продолжить. ", assistantVoiceCommandContinue},
		{"Хочу выполнить поручение", assistantVoiceCommandNone},
		{"Команда, выполни поручение", assistantVoiceCommandNone},
		{"Команда, продолжить сцену", assistantVoiceCommandNone},
	}
	for _, test := range tests {
		if got := parseAssistantVoiceCommand(test.text); got != test.want {
			t.Fatalf("parseAssistantVoiceCommand(%q)=%q, want %q", test.text, got, test.want)
		}
	}
}
