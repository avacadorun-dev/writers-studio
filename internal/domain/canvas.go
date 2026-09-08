package domain

import (
	"encoding/json"
	"time"
)

type CanvasSummary struct {
	ID              string     `json:"id"`
	ProjectID       string     `json:"projectId"`
	Title           string     `json:"title"`
	Position        float64    `json:"position"`
	CurrentRevision int        `json:"currentRevision"`
	TrashedAt       *time.Time `json:"trashedAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type CanvasRevision struct {
	ID            string          `json:"id"`
	CanvasID      string          `json:"canvasId"`
	Revision      int             `json:"revision"`
	BaseRevision  int             `json:"baseRevision"`
	SchemaVersion int             `json:"schemaVersion"`
	Content       json.RawMessage `json:"content"`
	Source        string          `json:"source"`
	CreatedAt     time.Time       `json:"createdAt"`
}

type CanvasDocumentPreview struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Kind            string     `json:"kind"`
	Status          string     `json:"status"`
	CurrentRevision int        `json:"currentRevision"`
	PlainText       string     `json:"plainText"`
	TrashedAt       *time.Time `json:"trashedAt,omitempty"`
}

type CanvasWorkspace struct {
	Canvas    CanvasSummary           `json:"canvas"`
	Revision  CanvasRevision          `json:"revision"`
	Documents []CanvasDocumentPreview `json:"documents"`
	Media     []ProjectMedia          `json:"media"`
}
