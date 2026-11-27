# Interface language

## Purpose

Make the web workspace usable in English, Russian, and Simplified Chinese.

## Requirements

### Requirement: Initial language

Writer's Studio SHALL use the saved interface language first, the first supported browser preference second, and English otherwise.

#### Scenario: Recognize a regional browser preference

- **WHEN** a browser lists Chinese as its first supported language and has no saved preference
- **THEN** the interface and its document language use Simplified Chinese

#### Scenario: Use English as a fallback

- **WHEN** the browser lists only unsupported languages and has no valid saved preference
- **THEN** the interface opens in English

### Requirement: Language selection

The sign-in and settings screens SHALL let users select English, Russian, or Simplified Chinese and apply their selection in the current browser.

#### Scenario: Switch and reopen the workspace

- **WHEN** the user selects a different language and applies it
- **THEN** the current page reloads with labels, controls and date and number formatting in the selected language
- **AND** subsequent visits in the same browser use that language

#### Scenario: Keep language choices independent

- **WHEN** a user changes the interface language while working on a Russian-language project
- **THEN** the project language, manuscript content, and speech-recognition choices retain their previous values

### Requirement: English reference material

Built-in help, help search and the OpenAPI reference SHALL use English independently of the interface preference.

#### Scenario: Read help after choosing another language

- **WHEN** an author using a Russian or Simplified Chinese interface opens built-in help
- **THEN** the help page and search use English
- **AND** returning to the workspace restores the selected interface language
