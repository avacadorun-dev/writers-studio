package export

import (
	"errors"
	"strings"

	"writers.studio/internal/domain"
)

var ErrInvalidProfile = errors.New("invalid export profile")

func DefaultProfile(title, synopsis string) domain.ExportProfile {
	return domain.ExportProfile{
		SchemaVersion: 1,
		Metadata: domain.ExportMetadata{
			Title: title, Annotation: synopsis,
		},
		Style: domain.ExportStyle{
			Preset: "book", FontFamily: "literata", FontSizePT: 11,
			LineHeight: 1.5, ParagraphMode: "indent", FirstLineIndent: 1.25,
			TextAlign: "justify", PageSize: "a5", MarginMM: 20,
			SceneSeparator: "asterisks",
		},
		TitlePage: true, TableOfContents: true, ChapterTitles: true,
		ChapterPageBreak: true, LastFormat: "docx",
	}
}

func NormalizeProfile(profile domain.ExportProfile, title, synopsis string) domain.ExportProfile {
	defaults := DefaultProfile(title, synopsis)
	if profile.SchemaVersion == 0 {
		return defaults
	}
	profile.Metadata.Title = strings.TrimSpace(profile.Metadata.Title)
	if profile.Metadata.Title == "" {
		profile.Metadata.Title = title
	}
	if profile.Metadata.Annotation == "" {
		profile.Metadata.Annotation = synopsis
	}
	return profile
}

func ValidateProfile(profile domain.ExportProfile) error {
	if profile.SchemaVersion != 1 || runeLength(profile.Metadata.Title) < 1 ||
		runeLength(profile.Metadata.Title) > 200 || runeLength(profile.Metadata.Subtitle) > 200 ||
		runeLength(profile.Metadata.Author) > 200 || runeLength(profile.Metadata.Annotation) > 4000 {
		return ErrInvalidProfile
	}
	if !oneOf(profile.Style.Preset, "book", "modern", "manuscript", "custom") ||
		!oneOf(profile.Style.FontFamily, "literata", "source-serif", "inter", "ibm-plex-mono") ||
		profile.Style.FontSizePT < 10 || profile.Style.FontSizePT > 16 ||
		profile.Style.LineHeight < 1.2 || profile.Style.LineHeight > 2 ||
		!oneOf(profile.Style.ParagraphMode, "indent", "spaced") ||
		profile.Style.FirstLineIndent < 0 || profile.Style.FirstLineIndent > 2 ||
		profile.Style.ParagraphSpace < 0 || profile.Style.ParagraphSpace > 1.5 ||
		!oneOf(profile.Style.TextAlign, "left", "justify") ||
		!oneOf(profile.Style.PageSize, "a4", "a5", "letter") ||
		profile.Style.MarginMM < 15 || profile.Style.MarginMM > 30 ||
		!oneOf(profile.Style.SceneSeparator, "asterisks", "dots", "line") ||
		!oneOf(profile.LastFormat, "docx", "epub", "pdf", "markdown", "txt") {
		return ErrInvalidProfile
	}
	return nil
}

func runeLength(value string) int { return len([]rune(strings.TrimSpace(value))) }

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
