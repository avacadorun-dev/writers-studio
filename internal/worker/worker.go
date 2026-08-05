package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	dictationdomain "writers.studio/internal/dictation"
	"writers.studio/internal/domain"
	"writers.studio/internal/editor"
	exportformat "writers.studio/internal/export"
	"writers.studio/internal/objectstore"
	"writers.studio/internal/store"
	"writers.studio/internal/stt"
)

type Worker struct {
	store        *store.Store
	objects      objectstore.Store
	transcriber  stt.FinalTranscriber
	liveProvider string
	liveModel    string
	logger       *slog.Logger
}

type finalizePayload struct {
	SessionID string `json:"sessionId"`
}

type exportPayload struct {
	ExportID string `json:"exportId"`
}

type manifest struct {
	Format string          `json:"format"`
	Chunks []manifestChunk `json:"chunks"`
}

type manifestChunk struct {
	Sequence int    `json:"sequence"`
	SHA256   string `json:"sha256"`
}

func New(
	dataStore *store.Store,
	objects objectstore.Store,
	transcriber stt.FinalTranscriber,
	liveProvider string,
	liveModel string,
	logger *slog.Logger,
) *Worker {
	return &Worker{
		store:        dataStore,
		objects:      objects,
		transcriber:  transcriber,
		liveProvider: liveProvider,
		liveModel:    liveModel,
		logger:       logger,
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for {
				job, err := w.store.ClaimBackgroundJob(ctx)
				if errors.Is(err, pgx.ErrNoRows) {
					break
				}
				if err != nil {
					w.logger.Error("claim background job", "error", err)
					break
				}
				w.process(ctx, job)
			}
		}
	}
}

func (w *Worker) process(ctx context.Context, job store.BackgroundJob) {
	switch job.Kind {
	case "finalize_dictation":
		w.processDictation(ctx, job)
	case "retranscribe_dictation":
		w.processRetranscription(ctx, job)
	case "render_export":
		w.processExport(ctx, job)
	default:
		w.logger.Error("unknown background job kind", "kind", job.Kind, "jobId", job.ID)
		_ = w.store.CompleteBackgroundJob(ctx, job.ID, json.RawMessage(`{"ignored":true}`))
	}
}

func (w *Worker) processRetranscription(ctx context.Context, job store.BackgroundJob) {
	var payload finalizePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		w.logger.Error("decode retranscription job", "jobId", job.ID, "error", err)
		_ = w.store.FailBackgroundJobOnly(ctx, job.ID, "invalid_payload")
		return
	}
	meta, err := w.store.GetWorkerSessionMeta(ctx, payload.SessionID)
	if err != nil {
		w.logger.Error("load retranscription session", "jobId", job.ID, "error", err)
		_ = w.store.FailBackgroundJobOnly(ctx, job.ID, "session_unavailable")
		return
	}
	asset, err := w.store.GetAudioAssetForWorker(ctx, payload.SessionID)
	if err != nil {
		w.logger.Error("load retranscription audio", "jobId", job.ID, "error", err)
		_ = w.store.FailBackgroundJobOnly(ctx, job.ID, "audio_unavailable")
		return
	}
	audioBytes, err := w.objects.Get(ctx, asset.StorageKey)
	if err != nil {
		w.logger.Error("read retranscription audio", "jobId", job.ID, "error", err)
		_ = w.store.FailBackgroundJobOnly(ctx, job.ID, "audio_unavailable")
		return
	}
	terms, err := w.store.ListDictionaryTerms(ctx, meta.OwnerID, meta.ProjectID)
	if err != nil {
		w.logger.Error("load retranscription dictionary", "jobId", job.ID, "error", err)
		_ = w.store.FailBackgroundJobOnly(ctx, job.ID, "dictionary_unavailable")
		return
	}
	activeTerms := make([]string, 0, len(terms))
	for _, term := range terms {
		if term.Active {
			activeTerms = append(activeTerms, term.Canonical)
		}
	}
	format := strings.TrimPrefix(path.Ext(asset.StorageKey), ".")
	if format == "" {
		format = "webm"
	}
	final, err := w.transcriber.Transcribe(ctx, audioBytes, stt.FinalConfig{
		SessionID:      payload.SessionID,
		Language:       meta.Language,
		Format:         format,
		DurationMS:     asset.DurationMS,
		DictionaryTerm: activeTerms,
	})
	if err != nil {
		w.logger.Warn("retranscription failed", "jobId", job.ID, "errorClass", "stt_unavailable")
		_ = w.store.FailBackgroundJobOnly(ctx, job.ID, "stt_unavailable")
		return
	}
	store.SortSegments(final.Segments)
	if err := normalizeFinalCommands(&final, meta); err != nil {
		w.logger.Error("normalize retranscription commands", "jobId", job.ID, "error", err)
		_ = w.store.FailBackgroundJobOnly(ctx, job.ID, "invalid_transcript")
		return
	}
	artifact, err := w.store.CreateTranscriptArtifact(ctx, domain.TranscriptArtifact{
		SessionID:       payload.SessionID,
		Kind:            "cleaned_suggestion",
		PipelineVersion: final.PipelineVersion,
		Provider:        final.Provider,
		Model:           final.Model,
		Language:        final.Language,
		Text:            final.Text,
		Segments:        final.Segments,
		AudioSHA256:     &asset.SHA256,
	})
	if err != nil {
		w.logger.Error("save retranscription artifact", "jobId", job.ID, "error", err)
		_ = w.store.FailBackgroundJobOnly(ctx, job.ID, "artifact_unavailable")
		return
	}
	result, _ := json.Marshal(map[string]string{
		"sessionId":  payload.SessionID,
		"artifactId": artifact.ID,
	})
	_ = w.store.CompleteBackgroundJob(ctx, job.ID, result)
}

func (w *Worker) processExport(ctx context.Context, job store.BackgroundJob) {
	startedAt := time.Now()
	var payload exportPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		w.logger.Error("decode export job", "jobId", job.ID, "error", err)
		return
	}
	exportJob, err := w.store.GetExportForWorker(ctx, payload.ExportID)
	if err != nil {
		w.logger.Error("load export job", "jobId", job.ID, "error", err)
		return
	}
	var snapshot exportformat.Snapshot
	if err := json.Unmarshal(exportJob.Snapshot, &snapshot); err != nil {
		_ = w.store.FailExport(ctx, payload.ExportID, "invalid_snapshot")
		_ = w.store.CompleteBackgroundJob(ctx, job.ID, json.RawMessage(`{"failed":true}`))
		return
	}
	if exportJob.Format == "markdown_tree" {
		w.processMarkdownTreeExport(ctx, job, exportJob, snapshot, startedAt)
		return
	}
	assets := make(map[string][]byte, len(snapshot.Media))
	for _, media := range snapshot.Media {
		data, mediaErr := w.objects.Get(ctx, media.StorageKey)
		if mediaErr != nil {
			w.logger.Warn("read export media", "exportId", payload.ExportID, "mediaId", media.ID, "error", mediaErr)
			_ = w.store.FailExport(ctx, payload.ExportID, "media_unavailable")
			_ = w.store.CompleteBackgroundJob(ctx, job.ID, json.RawMessage(`{"failed":true}`))
			return
		}
		assets[media.ID] = data
	}
	loadExportFonts(assets, os.Getenv("EXPORT_FONT_DIR"))
	data, contentType, err := exportformat.RenderWithAssets(
		ctx,
		exportJob.Format,
		snapshot,
		assets,
		os.Getenv("CHROMIUM_PATH"),
	)
	if err != nil {
		errorCode := "render_failed"
		if errors.Is(err, exportformat.ErrInvalidProfile) {
			errorCode = "invalid_export_profile"
		} else if errors.Is(err, exportformat.ErrPDFRendererUnavailable) {
			errorCode = "pdf_renderer_unavailable"
		}
		w.logger.Warn("render export", "exportId", payload.ExportID, "format", exportJob.Format, "errorCode", errorCode, "error", err)
		_ = w.store.FailExport(ctx, payload.ExportID, errorCode)
		_ = w.store.CompleteBackgroundJob(ctx, job.ID, json.RawMessage(`{"failed":true}`))
		return
	}
	extension := exportJob.Format
	if extension == "markdown" {
		extension = "md"
	} else if extension == "markdown_tree" {
		extension = "zip"
	}
	key := path.Join("exports", exportJob.ProjectID, exportJob.ID+"."+extension)
	if err := w.objects.Put(ctx, key, contentType, data); err != nil {
		_ = w.store.FailExport(ctx, payload.ExportID, "storage_failed")
		_ = w.store.CompleteBackgroundJob(ctx, job.ID, json.RawMessage(`{"failed":true}`))
		return
	}
	if err := w.store.CompleteExport(ctx, payload.ExportID, key); err != nil {
		w.logger.Error("complete export", "jobId", job.ID, "error", err)
		return
	}
	result, _ := json.Marshal(map[string]string{"exportId": payload.ExportID})
	_ = w.store.CompleteBackgroundJob(ctx, job.ID, result)
	w.logger.Info(
		"export rendered",
		"exportId", payload.ExportID,
		"format", exportJob.Format,
		"bytes", len(data),
		"duration", time.Since(startedAt),
	)
}

func (w *Worker) processMarkdownTreeExport(
	ctx context.Context,
	backgroundJob store.BackgroundJob,
	exportJob domain.ExportJob,
	snapshot exportformat.Snapshot,
	startedAt time.Time,
) {
	fail := func(code string, err error) {
		w.logger.Warn("render markdown tree", "exportId", exportJob.ID, "errorCode", code, "error", err)
		_ = w.store.FailExport(ctx, exportJob.ID, code)
		_ = w.store.CompleteBackgroundJob(ctx, backgroundJob.ID, json.RawMessage(`{"failed":true}`))
	}
	temporary, err := os.CreateTemp("", "writers-studio-markdown-*.zip")
	if err != nil {
		fail("render_failed", err)
		return
	}
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporary.Name())
	}()
	mediaUnavailable := false
	err = exportformat.RenderMarkdownTreeStream(
		temporary,
		snapshot,
		func(media exportformat.SnapshotMedia) ([]byte, error) {
			data, mediaErr := w.objects.Get(ctx, media.StorageKey)
			if mediaErr != nil {
				mediaUnavailable = true
				return nil, fmt.Errorf("read media %s: %w", media.ID, mediaErr)
			}
			return data, nil
		},
	)
	if err != nil {
		code := "render_failed"
		if mediaUnavailable {
			code = "media_unavailable"
		}
		fail(code, err)
		return
	}
	info, err := temporary.Stat()
	if err != nil {
		fail("render_failed", err)
		return
	}
	if _, err := temporary.Seek(0, 0); err != nil {
		fail("render_failed", err)
		return
	}
	key := path.Join("exports", exportJob.ProjectID, exportJob.ID+".zip")
	if err := w.objects.PutReader(ctx, key, "application/zip", temporary, info.Size()); err != nil {
		fail("storage_failed", err)
		return
	}
	if err := w.store.CompleteExport(ctx, exportJob.ID, key); err != nil {
		w.logger.Error("complete export", "jobId", backgroundJob.ID, "error", err)
		return
	}
	result, _ := json.Marshal(map[string]string{"exportId": exportJob.ID})
	_ = w.store.CompleteBackgroundJob(ctx, backgroundJob.ID, result)
	w.logger.Info(
		"export rendered",
		"exportId", exportJob.ID,
		"format", exportJob.Format,
		"bytes", info.Size(),
		"duration", time.Since(startedAt),
	)
}

func loadExportFonts(assets map[string][]byte, directory string) {
	if directory == "" {
		return
	}
	for _, family := range []string{"literata", "source-serif-4", "inter", "ibm-plex-mono"} {
		for _, script := range []string{"cyrillic", "latin"} {
			filename := family + "-" + script + "-400-normal.woff2"
			data, err := os.ReadFile(path.Join(directory, filename))
			if err == nil {
				assets["__export_font__"+filename] = data
			}
		}
	}
}

func (w *Worker) processDictation(ctx context.Context, job store.BackgroundJob) {
	var payload finalizePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		w.logger.Error("decode finalization job", "jobId", job.ID, "error", err)
		return
	}

	result, sessionStatus, err := w.finalizeDictation(ctx, payload.SessionID)
	if err != nil {
		errorCode := "finalization_failed"
		if errors.Is(err, errMissingChunks) {
			errorCode = "missing_chunks"
		}
		w.logger.Warn(
			"dictation finalization failed",
			"jobId",
			job.ID,
			"sessionId",
			payload.SessionID,
			"errorClass",
			errorCode,
		)
		_ = w.store.FailBackgroundJob(ctx, job.ID, payload.SessionID, errorCode, sessionStatus)
		return
	}
	resultJSON, _ := json.Marshal(result)
	if err := w.store.CompleteBackgroundJob(ctx, job.ID, resultJSON); err != nil {
		w.logger.Error("complete finalization job", "jobId", job.ID, "error", err)
	}
}

var errMissingChunks = errors.New("audio manifest has missing chunks")

func (w *Worker) finalizeDictation(
	ctx context.Context,
	sessionID string,
) (map[string]string, string, error) {
	meta, err := w.store.GetWorkerSessionMeta(ctx, sessionID)
	if err != nil {
		return nil, "finalization_failed", err
	}
	var expected manifest
	if err := json.Unmarshal(meta.Manifest, &expected); err != nil {
		return nil, "missing_chunks", errMissingChunks
	}
	if expected.Format == "" {
		expected.Format = "webm"
	}

	chunks, err := w.store.ListAudioChunks(ctx, sessionID)
	if err != nil {
		return nil, "finalization_failed", err
	}
	if len(expected.Chunks) != len(chunks) || len(chunks) == 0 {
		return nil, "missing_chunks", errMissingChunks
	}

	expectedBySequence := make(map[int]string, len(expected.Chunks))
	for _, chunk := range expected.Chunks {
		expectedBySequence[chunk.Sequence] = chunk.SHA256
	}

	var assembled bytes.Buffer
	for _, chunk := range chunks {
		if expectedSHA, ok := expectedBySequence[chunk.Sequence]; !ok || expectedSHA != chunk.SHA256 {
			return nil, "missing_chunks", errMissingChunks
		}
		data, err := w.objects.Get(ctx, chunk.StorageKey)
		if err != nil {
			return nil, "missing_chunks", errMissingChunks
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != chunk.SHA256 {
			return nil, "missing_chunks", fmt.Errorf("%w: checksum mismatch", errMissingChunks)
		}
		_, _ = assembled.Write(data)
	}

	audioBytes := assembled.Bytes()
	audioSum := sha256.Sum256(audioBytes)
	audioSHA := hex.EncodeToString(audioSum[:])
	assetKey := path.Join("audio", sessionID, "complete."+expected.Format)
	if err := w.objects.Put(ctx, assetKey, mediaType(expected.Format), audioBytes); err != nil {
		return nil, "finalization_failed", err
	}
	if err := w.store.SaveAudioAsset(ctx, store.AudioAsset{
		ID:         uuid.NewString(),
		SessionID:  sessionID,
		SHA256:     audioSHA,
		StorageKey: assetKey,
		ByteSize:   int64(len(audioBytes)),
		DurationMS: meta.RecordedDurationMS,
	}); err != nil {
		return nil, "finalization_failed", err
	}

	liveSegments, err := w.store.ListLiveSegments(ctx, sessionID)
	if err != nil {
		return nil, "finalization_failed", err
	}
	if w.liveProvider == "deepgram" && meta.LiveFinalizedAt != nil && len(liveSegments) > 0 {
		result, err := w.finalizeFromLiveTranscript(
			ctx,
			meta,
			sessionID,
			audioSHA,
			liveSegments,
		)
		if err != nil {
			return nil, "finalization_failed", err
		}
		return result, "review_ready", nil
	}

	terms, err := w.store.ListDictionaryTerms(ctx, meta.OwnerID, meta.ProjectID)
	if err != nil {
		return nil, "finalization_failed", err
	}
	activeTerms := make([]string, 0, len(terms))
	for _, term := range terms {
		if term.Active {
			activeTerms = append(activeTerms, term.Canonical)
		}
	}

	final, err := w.transcriber.Transcribe(ctx, audioBytes, stt.FinalConfig{
		SessionID:      sessionID,
		Language:       meta.Language,
		Format:         expected.Format,
		DurationMS:     meta.RecordedDurationMS,
		DictionaryTerm: activeTerms,
	})
	if err != nil {
		return nil, "finalization_failed", err
	}
	store.SortSegments(final.Segments)
	if err := normalizeFinalCommands(&final, meta); err != nil {
		return nil, "finalization_failed", err
	}
	batchArtifact, err := w.store.CreateTranscriptArtifact(ctx, domain.TranscriptArtifact{
		SessionID:       sessionID,
		Kind:            "batch_source",
		PipelineVersion: final.PipelineVersion,
		Provider:        final.Provider,
		Model:           final.Model,
		Language:        final.Language,
		Text:            final.Text,
		Segments:        final.Segments,
		AudioSHA256:     &audioSHA,
	})
	if err != nil {
		return nil, "finalization_failed", err
	}

	if len(liveSegments) > 0 {
		liveDocument, err := editor.FromTranscript(liveSegments)
		if err != nil {
			return nil, "finalization_failed", err
		}
		liveText, err := editor.PlainText(liveDocument)
		if err != nil {
			return nil, "finalization_failed", err
		}
		if _, err := w.store.CreateTranscriptArtifact(ctx, domain.TranscriptArtifact{
			SessionID:       sessionID,
			Kind:            "live_final",
			PipelineVersion: "normalized-live-v3",
			Provider:        w.liveProvider,
			Model:           w.liveModel,
			Language:        meta.Language,
			Text:            liveText,
			Segments:        liveSegments,
			AudioSHA256:     &audioSHA,
		}); err != nil {
			return nil, "finalization_failed", err
		}
	}

	document, err := editor.FromTranscript(final.Segments)
	if err != nil {
		return nil, "finalization_failed", err
	}
	if _, err := w.store.UpsertTranscriptDraft(ctx, domain.TranscriptDraft{
		SessionID:        sessionID,
		SourceArtifactID: &batchArtifact.ID,
		BaseSourceText:   final.Text,
		Content:          document,
		PlainText:        final.Text,
		Source:           "batch_source",
	}); err != nil {
		return nil, "finalization_failed", err
	}
	if err := w.store.FinalizeDictationState(ctx, sessionID, "review_ready"); err != nil {
		return nil, "finalization_failed", err
	}

	return map[string]string{
		"sessionId":  sessionID,
		"artifactId": batchArtifact.ID,
	}, "review_ready", nil
}

func (w *Worker) finalizeFromLiveTranscript(
	ctx context.Context,
	meta store.WorkerSessionMeta,
	sessionID string,
	audioSHA string,
	segments []domain.TranscriptSegment,
) (map[string]string, error) {
	segments = dictationdomain.RecognizeCommandsInTranscript(
		segments,
		meta.CommandPrefix,
		meta.CommandsEnabled,
	)
	document, err := editor.FromTranscript(segments)
	if err != nil {
		return nil, err
	}
	text, err := editor.PlainText(document)
	if err != nil {
		return nil, err
	}
	artifact, err := w.store.CreateTranscriptArtifact(ctx, domain.TranscriptArtifact{
		SessionID:       sessionID,
		Kind:            "live_final",
		PipelineVersion: "normalized-live-v3",
		Provider:        w.liveProvider,
		Model:           w.liveModel,
		Language:        meta.Language,
		Text:            text,
		Segments:        segments,
		AudioSHA256:     &audioSHA,
	})
	if err != nil {
		return nil, err
	}
	if _, err := w.store.GetTranscriptDraft(ctx, meta.OwnerID, sessionID); errors.Is(err, pgx.ErrNoRows) {
		if _, err := w.store.UpsertTranscriptDraft(ctx, domain.TranscriptDraft{
			SessionID:        sessionID,
			SourceArtifactID: &artifact.ID,
			BaseSourceText:   text,
			Content:          document,
			PlainText:        text,
			Source:           "live_source",
		}); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if err := w.store.FinalizeDictationState(ctx, sessionID, "review_ready"); err != nil {
		return nil, err
	}
	return map[string]string{
		"sessionId":  sessionID,
		"artifactId": artifact.ID,
		"source":     "live_source",
	}, nil
}

func normalizeFinalCommands(final *stt.FinalResult, meta store.WorkerSessionMeta) error {
	segments := dictationdomain.RecognizeCommandsInTranscript(
		final.Segments,
		meta.CommandPrefix,
		meta.CommandsEnabled,
	)
	final.Segments = segments
	document, err := editor.FromTranscript(segments)
	if err != nil {
		return err
	}
	final.Text, err = editor.PlainText(document)
	return err
}

func mediaType(format string) string {
	switch format {
	case "wav":
		return "audio/wav"
	case "mp3":
		return "audio/mpeg"
	case "ogg":
		return "audio/ogg"
	default:
		return "audio/webm"
	}
}
