# Dictation and recovery

## Purpose

Capture reviewable speech while keeping original audio and recognition separate from editorial work.

## Requirements

### Requirement: Capture preflight

Before recording, the author SHALL be able to choose an available microphone, destination, recognition language and command policy. Device denial, recognizer unavailability and missing input SHALL have distinct statuses.

#### Scenario: Start one recording

- **WHEN** the author starts capture with a usable microphone
- **THEN** the application shows recording status and prevents a second active capture

#### Scenario: Pause and resume

- **WHEN** the author pauses and then resumes
- **THEN** pending audio is preserved and the same session continues with distinct elapsed and captured times

### Requirement: Live recognition and voice controls

Live recognition SHALL distinguish provisional text from stable segments. Follow mode SHALL stop when the reader scrolls away. Voice commands SHALL execute only when enabled and explicitly prefixed, while quick buttons remain available.

#### Scenario: Execute a recognized command

- **WHEN** the recognizer produces an enabled, prefixed, supported command
- **THEN** the preview shows what happened and offers Undo
- **AND** source capture preserves the original command speech

#### Scenario: Speak a command with the feature disabled

- **WHEN** the recognizer hears the same phrase with command recognition disabled
- **THEN** the phrase remains ordinary dictated text

### Requirement: Independent local capture

Loss of the network or live recognition SHALL leave supported local capture running. Unacknowledged audio SHALL remain available to recovery and upload retry.

#### Scenario: Reconnect while recording

- **WHEN** a connection drops and later returns
- **THEN** recording continues locally during the outage and unacknowledged chunks are retried

#### Scenario: Recover after closing the tab

- **WHEN** the author reopens the workspace after leaving unfinished audio locally
- **THEN** the recovery screen offers available audio for download or recovery and requires explicit deletion

### Requirement: Durable finalization

Stopping SHALL be idempotent. Finalization SHALL continue after leaving the page. A failed finalization SHALL retain source audio, available draft, and retry.

#### Scenario: Complete a live Deepgram session

- **WHEN** finalization has a complete stable live result
- **THEN** the live result becomes the immutable source transcript without an automatic batch request

### Requirement: Review and manuscript transfer

Review SHALL support editing the draft, bounded source playback, and comparison with later recognition. Re-transcription SHALL create a separate result. Manuscript transfer SHALL require a destination and default to append.

#### Scenario: Review a late suggestion

- **WHEN** a new recognition artifact arrives after manual edits
- **THEN** the current draft remains intact until explicit acceptance
- **AND** selecting a comparison segment seeks to the corresponding source range

#### Scenario: Undo an explicit transfer

- **WHEN** the author transfers reviewed text and then chooses Undo
- **THEN** the previous manuscript is restored through a new revision while original audio and transcripts remain immutable
