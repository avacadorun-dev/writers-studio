package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"writers.studio/internal/assistant"
	dictationdomain "writers.studio/internal/dictation"
	"writers.studio/internal/domain"
	"writers.studio/internal/editor"
	"writers.studio/internal/store"
	"writers.studio/internal/stt"
)

type createDictationRequest struct {
	ProjectID         string  `json:"projectId"`
	DestinationNodeID *string `json:"destinationNodeId"`
	ClientSessionID   string  `json:"clientSessionId"`
	Language          string  `json:"language"`
}

func (s *Server) handleCreateDictation(response http.ResponseWriter, request *http.Request) {
	var input createDictationRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	if input.ProjectID == "" || input.ClientSessionID == "" ||
		(input.Language != "ru" && input.Language != "en") {
		writeError(response, http.StatusUnprocessableEntity, "invalid_dictation", "Проверьте место и язык записи")
		return
	}
	session, err := s.store.CreateDictationSession(
		request.Context(),
		currentUser(request.Context()).ID,
		input.ProjectID,
		input.DestinationNodeID,
		input.ClientSessionID,
		input.Language,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{"session": session})
}

func (s *Server) handleGetDictation(response http.ResponseWriter, request *http.Request) {
	session, err := s.store.GetDictationSession(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "sessionID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"session": session})
}

type dictationTransitionRequest struct {
	Status string `json:"status"`
}

func (s *Server) handleDictationTransition(response http.ResponseWriter, request *http.Request) {
	var input dictationTransitionRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	session, err := s.store.TransitionDictation(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "sessionID"),
		input.Status,
	)
	if err != nil {
		writeError(response, http.StatusConflict, "invalid_transition", "Состояние записи уже изменилось")
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"session": session})
}

func (s *Server) handleUploadChunk(response http.ResponseWriter, request *http.Request) {
	sequence, err := strconv.Atoi(chi.URLParam(request, "sequence"))
	if err != nil || sequence < 0 {
		writeError(response, http.StatusUnprocessableEntity, "invalid_sequence", "Некорректный номер аудиофрагмента")
		return
	}
	durationMS, err := strconv.Atoi(request.Header.Get("X-Audio-Duration-Ms"))
	if err != nil || durationMS < 0 {
		writeError(response, http.StatusUnprocessableEntity, "invalid_duration", "Некорректная длительность фрагмента")
		return
	}
	declaredSHA := strings.ToLower(strings.TrimSpace(request.Header.Get("X-Content-SHA256")))
	if len(declaredSHA) != 64 {
		writeError(response, http.StatusUnprocessableEntity, "invalid_checksum", "Некорректная контрольная сумма")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(response, request.Body, 16<<20))
	if err != nil {
		writeError(response, http.StatusRequestEntityTooLarge, "chunk_too_large", "Аудиофрагмент слишком большой")
		return
	}
	sum := sha256.Sum256(data)
	actualSHA := hex.EncodeToString(sum[:])
	if actualSHA != declaredSHA {
		writeError(response, http.StatusUnprocessableEntity, "checksum_mismatch", "Аудиофрагмент повреждён")
		return
	}

	sessionID := chi.URLParam(request, "sessionID")
	key := path.Join("audio", sessionID, "chunks", fmt.Sprintf("%08d-%s.bin", sequence, actualSHA))
	contentType := request.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if err := s.objects.Put(request.Context(), key, contentType, data); err != nil {
		writeError(response, http.StatusServiceUnavailable, "storage_unavailable", "Запись сохранена на устройстве. Повторим синхронизацию")
		return
	}
	duplicate, err := s.store.PutAudioChunk(
		request.Context(),
		currentUser(request.Context()).ID,
		store.AudioChunk{
			SessionID:  sessionID,
			Sequence:   sequence,
			SHA256:     actualSHA,
			DurationMS: durationMS,
			StorageKey: key,
			ByteSize:   int64(len(data)),
		},
	)
	if err != nil {
		writeError(response, http.StatusConflict, "chunk_conflict", "Контрольная сумма аудиофрагмента не совпадает")
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"sequence":  sequence,
		"sha256":    actualSHA,
		"duplicate": duplicate,
		"durable":   true,
	})
}

type stopDictationRequest struct {
	DurationMS         int64           `json:"durationMs"`
	RecordedDurationMS int64           `json:"recordedDurationMs"`
	Manifest           json.RawMessage `json:"manifest"`
}

func (s *Server) handleStopDictation(response http.ResponseWriter, request *http.Request) {
	var input stopDictationRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	if len(input.Manifest) == 0 || input.DurationMS < 0 || input.RecordedDurationMS < 0 {
		writeError(response, http.StatusUnprocessableEntity, "invalid_manifest", "Не удалось проверить запись")
		return
	}
	session, err := s.store.StopDictation(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "sessionID"),
		input.DurationMS,
		input.RecordedDurationMS,
		input.Manifest,
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	if err := s.ensureLiveDraft(request.Context(), session); err != nil {
		s.logger.Warn(
			"create live transcript draft",
			"sessionId",
			session.ID,
			"errorClass",
			"live_draft_unavailable",
		)
	} else {
		session, err = s.store.GetDictationSession(
			request.Context(),
			currentUser(request.Context()).ID,
			session.ID,
		)
		if err != nil {
			handleStoreError(response, err)
			return
		}
	}
	writeJSON(response, http.StatusAccepted, map[string]any{"session": session})
}

func (s *Server) ensureLiveDraft(
	ctx context.Context,
	session domain.DictationSession,
) error {
	segments, err := s.store.ListLiveSegments(ctx, session.ID)
	if err != nil || len(segments) == 0 {
		return err
	}
	meta, err := s.store.GetWorkerSessionMeta(ctx, session.ID)
	if err != nil {
		return err
	}
	segments = dictationdomain.RecognizeCommandsInTranscript(
		segments,
		meta.CommandPrefix,
		meta.CommandsEnabled,
	)
	document, err := editor.FromTranscript(segments)
	if err != nil {
		return err
	}
	text, err := editor.PlainText(document)
	if err != nil {
		return err
	}
	provider := s.config.LiveSTTAdapter
	model := "normalized-live"
	if provider == "deepgram" {
		model = s.config.DeepgramSTTModel
	}
	artifact, err := s.store.CreateTranscriptArtifact(ctx, domain.TranscriptArtifact{
		SessionID:       session.ID,
		Kind:            "live_final",
		PipelineVersion: "normalized-live-v3",
		Provider:        provider,
		Model:           model,
		Language:        session.Language,
		Text:            text,
		Segments:        segments,
	})
	if err != nil {
		return err
	}
	_, err = s.store.UpsertTranscriptDraft(ctx, domain.TranscriptDraft{
		SessionID:        session.ID,
		SourceArtifactID: &artifact.ID,
		BaseSourceText:   text,
		Content:          document,
		PlainText:        text,
		Source:           "live_source",
	})
	return err
}

func (s *Server) handleRetryDictation(response http.ResponseWriter, request *http.Request) {
	if err := s.store.RetryDictation(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "sessionID"),
	); err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]string{"status": "finalizing"})
}

func (s *Server) handleRetranscribeDictation(response http.ResponseWriter, request *http.Request) {
	requestID := uuid.NewString()
	if err := s.store.QueueRetranscription(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "sessionID"),
		requestID,
	); err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]string{
		"status":    "queued",
		"requestId": requestID,
	})
}

type saveTranscriptDraftRequest struct {
	Content json.RawMessage `json:"content"`
	Source  string          `json:"source"`
}

func (s *Server) handleSaveTranscriptDraft(response http.ResponseWriter, request *http.Request) {
	var input saveTranscriptDraftRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	if err := editor.Validate(input.Content); err != nil {
		writeError(response, http.StatusUnprocessableEntity, "invalid_document", err.Error())
		return
	}
	plainText, err := editor.PlainText(input.Content)
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "invalid_document", err.Error())
		return
	}
	draft, err := s.store.GetTranscriptDraft(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "sessionID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	draft.Content = input.Content
	draft.PlainText = plainText
	draft.ManuallyEdited = true
	draft.Source = map[string]string{
		"":                  "Ручная правка",
		"manual":            "Ручная правка",
		"final_batch_apply": "Применена финальная расшифровка",
		"final_batch_undo":  "Отменено применение финальной расшифровки",
	}[input.Source]
	if draft.Source == "" {
		writeError(response, http.StatusUnprocessableEntity, "invalid_source", "Неизвестный источник правки")
		return
	}
	draft, err = s.store.UpsertTranscriptDraft(request.Context(), draft)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"draft": draft})
}

type transferTranscriptRequest struct {
	NodeID       string          `json:"nodeId"`
	BaseRevision int             `json:"baseRevision"`
	Content      json.RawMessage `json:"content"`
}

func (s *Server) handleTransferTranscript(response http.ResponseWriter, request *http.Request) {
	var input transferTranscriptRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	if input.NodeID == "" || editor.Validate(input.Content) != nil {
		writeError(response, http.StatusUnprocessableEntity, "invalid_transfer", "Не удалось подготовить вставку")
		return
	}
	plainText, err := editor.PlainText(input.Content)
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "invalid_document", err.Error())
		return
	}
	operationID := newOperationID()
	revision, err := s.store.SaveRevision(
		request.Context(),
		currentUser(request.Context()).ID,
		input.NodeID,
		store.SaveRevisionInput{
			BaseRevision: input.BaseRevision,
			Content:      input.Content,
			PlainText:    plainText,
			Source:       "transcript_transfer",
			OperationID:  &operationID,
		},
	)
	if conflict, ok := store.AsConflict(err); ok {
		writeJSON(response, http.StatusConflict, map[string]any{
			"error": map[string]any{
				"code":            "revision_conflict",
				"message":         "Мы сохранили оба варианта",
				"currentRevision": conflict.CurrentRevision,
				"conflictNodeId":  conflict.ConflictNodeID,
			},
		})
		return
	}
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, map[string]any{
		"revision":    revision,
		"operationId": operationID,
		"undoable":    true,
	})
}

func (s *Server) handleDictationAudio(response http.ResponseWriter, request *http.Request) {
	asset, err := s.store.GetAudioAsset(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "sessionID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"url":        "/api/dictations/" + chi.URLParam(request, "sessionID") + "/audio/content",
		"durationMs": asset.DurationMS,
		"sha256":     asset.SHA256,
	})
}

func (s *Server) handleDictationAudioContent(response http.ResponseWriter, request *http.Request) {
	asset, err := s.store.GetAudioAsset(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "sessionID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	data, err := s.objects.Get(request.Context(), asset.StorageKey)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	response.Header().Set("Content-Type", mediaTypeForStorageKey(asset.StorageKey))
	response.Header().Set("Cache-Control", "private, no-store")
	response.Header().Set("Content-Length", strconv.Itoa(len(data)))
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(data)
}

func (s *Server) handleSegmentAudioSource(response http.ResponseWriter, request *http.Request) {
	source, err := s.store.GetSegmentAudioSource(
		request.Context(),
		currentUser(request.Context()).ID,
		chi.URLParam(request, "segmentID"),
	)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"url":     "/api/dictations/" + source.SessionID + "/audio/content",
		"startMs": source.StartMS,
		"endMs":   source.EndMS,
	})
}

func mediaTypeForStorageKey(key string) string {
	switch strings.ToLower(path.Ext(key)) {
	case ".wav":
		return "audio/wav"
	case ".mp3":
		return "audio/mpeg"
	case ".ogg":
		return "audio/ogg"
	default:
		return "audio/webm"
	}
}

func (s *Server) handleRecovery(response http.ResponseWriter, request *http.Request) {
	items, err := s.store.ListRecoveryItems(request.Context(), currentUser(request.Context()).ID)
	if err != nil {
		handleStoreError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"items": items})
}

type liveClientMessage struct {
	Type                string                         `json:"type"`
	Text                string                         `json:"text,omitempty"`
	Sequence            int                            `json:"sequence,omitempty"`
	Format              string                         `json:"format,omitempty"`
	SampleRate          int                            `json:"sampleRate,omitempty"`
	StartMS             int64                          `json:"startMs,omitempty"`
	EndMS               int64                          `json:"endMs,omitempty"`
	Confidence          *float64                       `json:"confidence,omitempty"`
	CommandsEnabled     bool                           `json:"commandsEnabled,omitempty"`
	CommandPrefix       string                         `json:"commandPrefix,omitempty"`
	SegmentID           string                         `json:"segmentId,omitempty"`
	SegmentIDs          []string                       `json:"segmentIds,omitempty"`
	CommandKind         string                         `json:"commandKind,omitempty"`
	Executed            bool                           `json:"executed,omitempty"`
	SelectionAttachment *assistant.SelectionAttachment `json:"selectionAttachment,omitempty"`
}

type liveCommandState struct {
	mu      sync.RWMutex
	enabled bool
	prefix  string
}

func newLiveCommandState() *liveCommandState {
	return &liveCommandState{enabled: true, prefix: "Команда"}
}

func (s *liveCommandState) set(enabled bool, prefix string) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "Команда"
	}
	s.mu.Lock()
	s.enabled = enabled
	s.prefix = prefix
	s.mu.Unlock()
}

func (s *liveCommandState) get() (bool, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled, s.prefix
}

func (s *Server) handleLiveWebSocket(response http.ResponseWriter, request *http.Request) {
	connection, err := websocket.Accept(response, request, nil)
	if err != nil {
		return
	}
	defer connection.Close(websocket.StatusNormalClosure, "")
	connection.SetReadLimit(4 << 20)

	sessionID := chi.URLParam(request, "sessionID")
	user := currentUser(request.Context())
	session, err := s.store.GetDictationSession(request.Context(), user.ID, sessionID)
	if err != nil {
		_ = connection.Close(websocket.StatusPolicyViolation, "unknown session")
		return
	}

	activeTerms := make([]string, 0)
	if s.config.LiveSTTAdapter != "fake" {
		terms, listErr := s.store.ListDictionaryTerms(
			request.Context(),
			user.ID,
			session.ProjectID,
		)
		if listErr != nil {
			s.logger.Warn(
				"load live transcription dictionary",
				"sessionId",
				sessionID,
				"errorClass",
				"dictionary_unavailable",
			)
		} else {
			for _, term := range terms {
				if term.Active {
					activeTerms = append(activeTerms, term.Canonical)
				}
			}
		}
	}

	fixtureIndex := 0
	commandState := newLiveCommandState()
	var pendingAudio *liveClientMessage
	var liveProvider stt.LiveSession
	var liveProviderOffsetMS int64
	providerFlushed := make(chan struct{}, 1)
	providerContext, cancelProvider := context.WithCancel(request.Context())
	defer func() {
		cancelProvider()
		if liveProvider != nil {
			liveProvider.Close()
		}
	}()
	for {
		messageType, data, err := connection.Read(request.Context())
		if err != nil {
			return
		}
		if messageType == websocket.MessageBinary {
			if s.config.LiveSTTAdapter == "fake" {
				fixtureIndex++
				commandsEnabled, commandPrefix := commandState.get()
				if err := s.emitFixtureLive(
					request,
					connection,
					sessionID,
					fixtureIndex,
					commandsEnabled,
					commandPrefix,
				); err != nil {
					return
				}
				continue
			}
			if pendingAudio == nil {
				if err := writeLiveDegraded(
					request,
					connection,
					sessionID,
					"invalid_audio_chunk",
				); err != nil {
					return
				}
				continue
			}
			metadata := *pendingAudio
			pendingAudio = nil
			if s.config.LiveSTTAdapter == "deepgram" {
				if liveProvider == nil {
					if s.liveSTT == nil {
						_ = writeLiveDegraded(
							request,
							connection,
							sessionID,
							"stt_unavailable",
						)
						return
					}
					commandsEnabled, commandPrefix := commandState.get()
					providerTerms := append(
						dictationdomain.CommandKeyterms(commandPrefix, commandsEnabled),
						activeTerms...,
					)
					if err := s.store.MarkLiveTranscriptionStarted(
						request.Context(),
						user.ID,
						sessionID,
					); err != nil {
						s.logger.Warn(
							"mark live transcription started",
							"sessionId",
							sessionID,
							"errorClass",
							"live_state_unavailable",
						)
						_ = writeLiveDegraded(
							request,
							connection,
							sessionID,
							"stt_unavailable",
						)
						return
					}
					liveProvider, err = s.liveSTT.Open(request.Context(), stt.LiveConfig{
						Language:        session.Language,
						SampleRate:      metadata.SampleRate,
						DictionaryTerms: providerTerms,
					})
					if err != nil {
						s.logger.Warn(
							"connect live transcription",
							"sessionId",
							sessionID,
							"errorClass",
							"stt_unavailable",
						)
						_ = writeLiveDegraded(
							request,
							connection,
							sessionID,
							"stt_unavailable",
						)
						return
					}
					liveProviderOffsetMS = metadata.StartMS
					go s.forwardLiveProviderEvents(
						request,
						connection,
						sessionID,
						liveProvider,
						providerContext,
						liveProviderOffsetMS,
						commandState,
						providerFlushed,
					)
				}
				sendContext, cancel := context.WithTimeout(request.Context(), 5*time.Second)
				sendErr := liveProvider.SendAudio(sendContext, data)
				cancel()
				if sendErr != nil {
					s.logger.Warn(
						"send live audio",
						"sessionId",
						sessionID,
						"errorClass",
						"stt_unavailable",
					)
					_ = writeLiveDegraded(
						request,
						connection,
						sessionID,
						"stt_unavailable",
					)
					return
				}
				continue
			}
			commandsEnabled, commandPrefix := commandState.get()
			if err := s.transcribeAndSendLiveChunk(
				request,
				connection,
				session,
				metadata,
				data,
				activeTerms,
				commandsEnabled,
				commandPrefix,
			); err != nil {
				return
			}
			continue
		}
		if messageType != websocket.MessageText {
			continue
		}

		var message liveClientMessage
		if json.Unmarshal(data, &message) != nil {
			continue
		}
		switch message.Type {
		case "configure":
			commandState.set(message.CommandsEnabled, message.CommandPrefix)
			if err := s.store.ConfigureLiveCommands(
				request.Context(),
				user.ID,
				sessionID,
				message.CommandsEnabled,
				message.CommandPrefix,
			); err != nil {
				s.logger.Warn(
					"save live command settings",
					"sessionId",
					sessionID,
					"errorClass",
					"command_state_unavailable",
				)
			}
		case "fixture_segment":
			if s.config.LiveSTTAdapter != "fake" {
				continue
			}
			commandsEnabled, commandPrefix := commandState.get()
			segment := domain.TranscriptSegment{
				ID:         uuid.NewString(),
				Text:       strings.TrimSpace(message.Text),
				StartMS:    message.StartMS,
				EndMS:      message.EndMS,
				Confidence: message.Confidence,
				Command: dictationdomain.ParseCommand(
					message.Text,
					commandPrefix,
					commandsEnabled,
					message.Confidence,
				),
			}
			if err := s.persistAndSendSegment(
				request,
				connection,
				sessionID,
				"fixture-turn",
				segment,
			); err != nil {
				return
			}
		case "audio_chunk":
			validAudioChunk := message.Sequence >= 0 &&
				message.StartMS >= 0 &&
				message.EndMS > message.StartMS &&
				message.EndMS-message.StartMS <= 15_000
			switch s.config.LiveSTTAdapter {
			case "openrouter":
				validAudioChunk = validAudioChunk &&
					message.Format == "wav" &&
					message.SampleRate >= 8_000 &&
					message.SampleRate <= 48_000
			case "deepgram":
				validAudioChunk = validAudioChunk &&
					message.Format == "linear16" &&
					message.SampleRate >= 8_000 &&
					message.SampleRate <= 48_000
			default:
				validAudioChunk = false
			}
			if !validAudioChunk {
				if err := writeLiveDegraded(
					request,
					connection,
					sessionID,
					"invalid_audio_chunk",
				); err != nil {
					return
				}
				continue
			}
			pendingAudio = &message
		case "flush_live":
			if liveProvider != nil {
				select {
				case <-providerFlushed:
				default:
				}
				finalizeContext, cancel := context.WithTimeout(
					request.Context(),
					4*time.Second,
				)
				finalizeErr := liveProvider.Finalize(finalizeContext)
				cancel()
				if finalizeErr != nil {
					s.logger.Warn(
						"finalize live transcription",
						"sessionId",
						sessionID,
						"errorClass",
						"stt_unavailable",
					)
				} else {
					select {
					case <-providerFlushed:
						if err := s.store.MarkLiveTranscriptionFinalized(
							request.Context(),
							user.ID,
							sessionID,
						); err != nil {
							s.logger.Warn(
								"mark live transcription finalized",
								"sessionId",
								sessionID,
								"errorClass",
								"live_state_unavailable",
							)
						}
					case <-time.After(time.Second):
						s.logger.Warn(
							"wait for finalized live transcript persistence",
							"sessionId",
							sessionID,
							"errorClass",
							"live_state_unavailable",
						)
					}
				}
			}
			if err := writeWebSocketJSON(request.Context(), connection, map[string]any{
				"type":      "live_flushed",
				"sessionId": sessionID,
			}); err != nil {
				return
			}
		case "client_marker":
			segment := domain.TranscriptSegment{
				ID:      message.SegmentID,
				Text:    "Метка",
				StartMS: message.StartMS,
				EndMS:   message.EndMS,
				Command: &domain.CommandEvent{
					Kind:     "marker",
					RawText:  "Команда: метка",
					Executed: true,
				},
			}
			if segment.ID == "" {
				segment.ID = uuid.NewString()
			}
			if err := s.persistAndSendSegment(
				request,
				connection,
				sessionID,
				"client-marker",
				segment,
			); err != nil {
				return
			}
		case "client_command":
			label, valid := quickDictationCommandLabel(message.CommandKind)
			if !valid {
				continue
			}
			segment := domain.TranscriptSegment{
				ID:      message.SegmentID,
				Text:    label,
				StartMS: message.StartMS,
				EndMS:   message.EndMS,
				Command: &domain.CommandEvent{
					Kind:     message.CommandKind,
					RawText:  "Кнопка: " + label,
					Executed: true,
				},
			}
			if segment.ID == "" {
				segment.ID = uuid.NewString()
			}
			if err := s.persistAndSendSegment(
				request,
				connection,
				sessionID,
				"client-command",
				segment,
			); err != nil {
				return
			}
		case "set_command":
			if err := s.store.SetLiveCommandExecution(
				request.Context(),
				user.ID,
				sessionID,
				message.SegmentID,
				message.Executed,
			); err != nil {
				continue
			}
			_ = writeWebSocketJSON(request.Context(), connection, map[string]any{
				"type":      "command_updated",
				"sessionId": sessionID,
				"segmentId": message.SegmentID,
				"executed":  message.Executed,
			})
		case "edit_segment":
			text := strings.TrimSpace(message.Text)
			if text == "" {
				continue
			}
			if err := s.store.EditLiveSegment(
				request.Context(),
				user.ID,
				sessionID,
				message.SegmentID,
				text,
			); err != nil {
				continue
			}
			_ = writeWebSocketJSON(request.Context(), connection, map[string]any{
				"type":      "segment_updated",
				"sessionId": sessionID,
				"segmentId": message.SegmentID,
				"text":      text,
			})
		case "replace_segments":
			text := strings.TrimSpace(message.Text)
			if err := s.store.ReplaceLiveSegments(
				request.Context(),
				user.ID,
				sessionID,
				message.SegmentIDs,
				text,
			); err != nil {
				continue
			}
			_ = writeWebSocketJSON(request.Context(), connection, map[string]any{
				"type":       "segments_replaced",
				"sessionId":  sessionID,
				"segmentIds": message.SegmentIDs,
				"text":       text,
			})
		case "ping":
			_ = writeWebSocketJSON(request.Context(), connection, map[string]string{"type": "pong"})
		}
	}
}

func quickDictationCommandLabel(kind string) (string, bool) {
	labels := map[string]string{
		"new_paragraph":  "Новый абзац",
		"dialogue_start": "Начало диалога",
		"scene_break":    "Разделитель сцены",
		"undo_command":   "Отменить команду",
	}
	label, exists := labels[kind]
	return label, exists
}

func (s *Server) forwardLiveProviderEvents(
	request *http.Request,
	connection *websocket.Conn,
	sessionID string,
	provider stt.LiveSession,
	providerContext context.Context,
	offsetMS int64,
	commandState *liveCommandState,
	providerFlushed chan<- struct{},
) {
	for {
		event, err := provider.ReadEvent(providerContext)
		if err != nil {
			if providerContext.Err() == nil {
				s.logger.Warn(
					"read live transcription",
					"sessionId",
					sessionID,
					"errorClass",
					"stt_unavailable",
				)
				_ = writeLiveDegraded(
					request,
					connection,
					sessionID,
					"stt_unavailable",
				)
				_ = connection.Close(
					websocket.StatusTryAgainLater,
					"live transcription unavailable",
				)
			}
			return
		}
		startMS := offsetMS + event.StartMS
		endMS := offsetMS + event.EndMS
		signalFlushed := func() {
			if !event.FromFinalize {
				return
			}
			select {
			case providerFlushed <- struct{}{}:
			default:
			}
		}
		switch event.Kind {
		case "finalized":
			signalFlushed()
		case "speech_started", "speech_stopped":
			if err := writeWebSocketJSON(request.Context(), connection, map[string]any{
				"type":      event.Kind,
				"sessionId": sessionID,
				"startMs":   startMS,
				"endMs":     endMS,
			}); err != nil {
				return
			}
			signalFlushed()
		case "partial":
			if err := writeWebSocketJSON(request.Context(), connection, map[string]any{
				"type":           "partial",
				"sessionId":      sessionID,
				"providerTurnId": event.ProviderTurnID,
				"text":           event.Text,
				"startMs":        startMS,
				"endMs":          endMS,
			}); err != nil {
				return
			}
			signalFlushed()
		case "final":
			words := make([]domain.TranscriptWord, len(event.Words))
			for index, word := range event.Words {
				word.StartMS += offsetMS
				word.EndMS += offsetMS
				words[index] = word
			}
			if endMS <= startMS {
				endMS = startMS + 1
			}
			commandsEnabled, commandPrefix := commandState.get()
			segment := domain.TranscriptSegment{
				ID:         uuid.NewString(),
				Text:       event.Text,
				StartMS:    startMS,
				EndMS:      endMS,
				Confidence: event.Confidence,
				Words:      words,
				Command: dictationdomain.ParseCommand(
					event.Text,
					commandPrefix,
					commandsEnabled,
					event.Confidence,
				),
			}
			if err := s.persistAndSendSegment(
				request,
				connection,
				sessionID,
				event.ProviderTurnID,
				segment,
			); err != nil {
				return
			}
			signalFlushed()
		}
	}
}

func (s *Server) transcribeAndSendLiveChunk(
	request *http.Request,
	connection *websocket.Conn,
	session domain.DictationSession,
	metadata liveClientMessage,
	audio []byte,
	dictionaryTerms []string,
	commandsEnabled bool,
	commandPrefix string,
) error {
	if len(audio) < 44 {
		return writeLiveDegraded(request, connection, session.ID, "invalid_audio_chunk")
	}

	ctx, cancel := context.WithTimeout(request.Context(), 30*time.Second)
	defer cancel()
	result, err := s.transcriber.Transcribe(ctx, audio, stt.FinalConfig{
		SessionID:      fmt.Sprintf("%s-live-%d", session.ID, metadata.Sequence),
		Language:       session.Language,
		Format:         metadata.Format,
		DurationMS:     metadata.EndMS - metadata.StartMS,
		DictionaryTerm: dictionaryTerms,
	})
	if err != nil {
		s.logger.Warn(
			"live transcription failed",
			"sessionId",
			session.ID,
			"sequence",
			metadata.Sequence,
			"errorClass",
			"stt_unavailable",
		)
		return writeLiveDegraded(request, connection, session.ID, "stt_unavailable")
	}

	text := strings.TrimSpace(result.Text)
	if text == "" {
		return nil
	}
	segment := domain.TranscriptSegment{
		ID:      uuid.NewString(),
		Text:    text,
		StartMS: metadata.StartMS,
		EndMS:   metadata.EndMS,
		Command: dictationdomain.ParseCommand(
			text,
			commandPrefix,
			commandsEnabled,
			nil,
		),
	}
	providerTurnID := fmt.Sprintf("openrouter-live-%d", metadata.Sequence)
	return s.persistAndSendSegment(
		request,
		connection,
		session.ID,
		providerTurnID,
		segment,
	)
}

func writeLiveDegraded(
	request *http.Request,
	connection *websocket.Conn,
	sessionID string,
	reason string,
) error {
	return writeWebSocketJSON(request.Context(), connection, map[string]any{
		"type":      "stream_degraded",
		"sessionId": sessionID,
		"reason":    reason,
	})
}

func (s *Server) emitFixtureLive(
	request *http.Request,
	connection *websocket.Conn,
	sessionID string,
	index int,
	commandsEnabled bool,
	commandPrefix string,
) error {
	fixtures := []domain.TranscriptSegment{
		{
			ID:      "fixture-live-1",
			Text:    "Дом стоял у самой воды. Туман поднимался от реки и скрывал дальний берег.",
			StartMS: 0,
			EndMS:   4200,
		},
		{
			ID:      "fixture-live-2",
			Text:    "Анна остановилась у калитки и долго не решалась войти.",
			StartMS: 4300,
			EndMS:   7800,
		},
		{
			ID:      "fixture-live-3",
			Text:    "И тогда она поняла, что дом всё это время ждал именно её.",
			StartMS: 7900,
			EndMS:   11800,
		},
	}
	fixtureIndex := min((index-1)/2, len(fixtures)-1)
	fixture := fixtures[fixtureIndex]
	fixture.Command = dictationdomain.ParseCommand(
		fixture.Text,
		commandPrefix,
		commandsEnabled,
		fixture.Confidence,
	)
	if index%2 == 1 {
		partial := fixture.Text
		runes := []rune(partial)
		if len(runes) > 32 {
			partial = string(runes[:32]) + "…"
		}
		return writeWebSocketJSON(request.Context(), connection, map[string]any{
			"type":           "partial",
			"sessionId":      sessionID,
			"providerTurnId": "fixture-turn",
			"text":           partial,
			"startMs":        fixture.StartMS,
			"endMs":          fixture.EndMS,
		})
	}
	return s.persistAndSendSegment(
		request,
		connection,
		sessionID,
		"fixture-turn",
		fixture,
	)
}

func (s *Server) persistAndSendSegment(
	request *http.Request,
	connection *websocket.Conn,
	sessionID string,
	providerTurnID string,
	segment domain.TranscriptSegment,
) error {
	if segment.Command != nil &&
		segment.Command.Kind == "undo_command" &&
		segment.Command.Executed {
		undoneSegmentID, err := s.store.UndoLastLiveCommand(
			request.Context(),
			currentUser(request.Context()).ID,
			sessionID,
		)
		if err == nil {
			if err := writeWebSocketJSON(request.Context(), connection, map[string]any{
				"type":      "command_updated",
				"sessionId": sessionID,
				"segmentId": undoneSegmentID,
				"executed":  false,
			}); err != nil {
				return err
			}
		}
	}
	if err := s.store.AppendLiveSegment(
		request.Context(),
		currentUser(request.Context()).ID,
		sessionID,
		segment.ID,
		providerTurnID,
		segment.Text,
		segment.StartMS,
		segment.EndMS,
		segment.Words,
		segment.Command,
	); err != nil {
		return err
	}
	return writeWebSocketJSON(request.Context(), connection, map[string]any{
		"type":           "segment_final",
		"sessionId":      sessionID,
		"providerTurnId": providerTurnID,
		"segmentId":      segment.ID,
		"text":           segment.Text,
		"startMs":        segment.StartMS,
		"endMs":          segment.EndMS,
		"words":          segment.Words,
		"command":        segment.Command,
	})
}

func writeWebSocketJSON(ctx context.Context, connection *websocket.Conn, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return connection.Write(ctx, websocket.MessageText, data)
}
