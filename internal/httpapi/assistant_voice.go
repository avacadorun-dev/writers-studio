package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"writers.studio/internal/assistant"
	"writers.studio/internal/stt"
)

type assistantVoiceWriter struct {
	mu         sync.Mutex
	connection *websocket.Conn
}

func (w *assistantVoiceWriter) write(ctx context.Context, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.connection.Write(ctx, websocket.MessageText, encoded)
}

type assistantVoiceState struct {
	mu           sync.Mutex
	runID        string
	ownerID      string
	mode         assistant.InputMode
	turn         *assistant.Turn
	selection    *assistant.SelectionAttachment
	segments     []string
	silenceTimer *time.Timer
	checkpoint   time.Duration
	writer       *assistantVoiceWriter
	persistTurn  func(context.Context, assistant.Turn) error
	finalized    chan struct{}
}

func (v *assistantVoiceState) transcript() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.turn != nil {
		return strings.TrimSpace(v.turn.Transcript)
	}
	return strings.TrimSpace(strings.Join(v.segments, " "))
}

func (v *assistantVoiceState) resetSilenceTimer(ctx context.Context) {
	if v.checkpoint <= 0 {
		return
	}
	v.mu.Lock()
	if v.silenceTimer != nil {
		v.silenceTimer.Stop()
	}
	v.silenceTimer = time.AfterFunc(v.checkpoint, func() {
		writeContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		text, turnID, err := v.markAwaitingConfirmation(writeContext)
		if err != nil {
			_ = v.writer.write(writeContext, map[string]any{
				"type": "stream_degraded", "reason": "persistence_unavailable",
			})
			return
		}
		if text == "" {
			return
		}
		_ = v.writer.write(writeContext, map[string]any{
			"type":       "confirmation_required",
			"runId":      v.runID,
			"turnId":     turnID,
			"transcript": text,
			"prompt":     "Выполнить действие?",
		})
	})
	v.mu.Unlock()
}

func (v *assistantVoiceState) markAwaitingConfirmation(
	ctx context.Context,
) (string, string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.turn == nil || strings.TrimSpace(v.turn.Transcript) == "" {
		return "", "", nil
	}
	if v.turn.State != assistant.TurnCollecting &&
		v.turn.State != assistant.TurnAwaitingConfirmation {
		return "", "", nil
	}
	v.turn.State = assistant.TurnAwaitingConfirmation
	if v.persistTurn != nil {
		if err := v.persistTurn(ctx, *v.turn); err != nil {
			return "", "", err
		}
	}
	return strings.TrimSpace(v.turn.Transcript), v.turn.ID, nil
}

func voiceTurnID(state *assistantVoiceState) any {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.turn == nil {
		return nil
	}
	return state.turn.ID
}

func (v *assistantVoiceState) stopTimer() {
	v.mu.Lock()
	if v.silenceTimer != nil {
		v.silenceTimer.Stop()
		v.silenceTimer = nil
	}
	v.mu.Unlock()
}

func (s *Server) handleAssistantVoiceWebSocket(
	response http.ResponseWriter,
	request *http.Request,
) {
	connection, err := websocket.Accept(response, request, nil)
	if err != nil {
		return
	}
	defer connection.Close(websocket.StatusNormalClosure, "")
	connection.SetReadLimit(4 << 20)
	writer := &assistantVoiceWriter{connection: connection}
	user := currentUser(request.Context())
	runID := chiURLParam(request, "runID")
	run, err := s.store.GetAssistantRun(request.Context(), user.ID, runID)
	if err != nil ||
		(run.Status != assistant.RunActive && run.Status != assistant.RunWorking) {
		_ = connection.Close(websocket.StatusPolicyViolation, "voice run unavailable")
		return
	}
	workspace, err := s.store.GetProject(request.Context(), user.ID, run.ProjectID)
	if err != nil {
		_ = connection.Close(websocket.StatusPolicyViolation, "project unavailable")
		return
	}
	checkpoint := time.Duration(run.SilenceCheckpointMS) * time.Millisecond
	state := &assistantVoiceState{
		runID: run.ID, ownerID: user.ID, mode: assistant.InputVoice,
		checkpoint: checkpoint, writer: writer, finalized: make(chan struct{}, 1),
		persistTurn: s.store.UpdateAssistantTurn,
	}
	turns, err := s.store.ListAssistantTurns(request.Context(), user.ID, run.ID)
	if err != nil {
		_ = connection.Close(websocket.StatusInternalError, "voice draft unavailable")
		return
	}
	for index := len(turns) - 1; index >= 0; index-- {
		candidate := turns[index]
		if candidate.InputMode == assistant.InputVoice &&
			(candidate.State == assistant.TurnCollecting ||
				candidate.State == assistant.TurnAwaitingConfirmation) {
			state.turn = &candidate
			state.selection = candidate.SelectionAttachment
			if text := strings.TrimSpace(candidate.Transcript); text != "" {
				state.segments = []string{text}
			}
			_ = writer.write(request.Context(), map[string]any{
				"type": "draft_restored", "turnId": candidate.ID,
				"transcript": candidate.Transcript, "state": candidate.State,
			})
			break
		}
	}
	_ = writer.write(request.Context(), map[string]any{"type": "voice_ready"})
	defer state.stopTimer()

	terms, _ := s.store.ListDictionaryTerms(request.Context(), user.ID, run.ProjectID)
	dictionary := make([]string, 0, len(terms))
	for _, term := range terms {
		if term.Active {
			dictionary = append(dictionary, term.Canonical)
		}
	}
	var provider stt.LiveSession
	var providerCancel context.CancelFunc
	var pending *liveClientMessage
	fixtureIndex := 0
	defer func() {
		if providerCancel != nil {
			providerCancel()
		}
		if provider != nil {
			provider.Close()
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
				text := []string{
					"Напиши сцену встречи у старого дома.",
					"Сделай диалог менее прямолинейным, но не меняй последнюю реплику.",
				}[(fixtureIndex-1)%2]
				if err := s.appendAssistantVoiceText(
					request.Context(),
					state,
					"fixture-"+uuid.NewString(),
					text,
					int64((fixtureIndex-1)*3000),
					int64(fixtureIndex*3000),
					nil,
				); err != nil {
					return
				}
				continue
			}
			if pending == nil {
				_ = writer.write(request.Context(), map[string]any{
					"type": "stream_degraded", "reason": "invalid_audio_chunk",
				})
				continue
			}
			metadata := *pending
			pending = nil
			if provider == nil {
				if s.liveSTT == nil {
					_ = writer.write(request.Context(), map[string]any{
						"type": "stream_degraded", "reason": "stt_unavailable",
					})
					return
				}
				provider, err = s.liveSTT.Open(request.Context(), stt.LiveConfig{
					Language: workspace.Project.Language, SampleRate: metadata.SampleRate,
					DictionaryTerms: dictionary,
				})
				if err != nil {
					_ = writer.write(request.Context(), map[string]any{
						"type": "stream_degraded", "reason": "stt_unavailable",
					})
					return
				}
				var providerContext context.Context
				providerContext, providerCancel = context.WithCancel(request.Context())
				go s.forwardAssistantVoiceEvents(providerContext, state, provider)
			}
			sendContext, cancel := context.WithTimeout(request.Context(), 5*time.Second)
			err = provider.SendAudio(sendContext, data)
			cancel()
			if err != nil {
				_ = writer.write(request.Context(), map[string]any{
					"type": "stream_degraded", "reason": "stt_unavailable",
				})
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
		case "set_selection":
			if err := s.setAssistantVoiceSelection(
				request.Context(), state, message.SelectionAttachment,
			); err != nil {
				_ = writer.write(request.Context(), map[string]any{
					"type": "selection_rejected", "reason": "invalid_selection",
				})
				continue
			}
			_ = writer.write(request.Context(), map[string]any{"type": "selection_updated"})
		case "audio_chunk":
			valid := message.Sequence >= 0 && message.StartMS >= 0 &&
				message.EndMS > message.StartMS &&
				message.EndMS-message.StartMS <= 15_000 &&
				message.Format == "linear16" &&
				message.SampleRate >= 8_000 && message.SampleRate <= 48_000
			if !valid && s.config.LiveSTTAdapter != "fake" {
				_ = writer.write(request.Context(), map[string]any{
					"type": "stream_degraded", "reason": "invalid_audio_chunk",
				})
				continue
			}
			pending = &message
		case "fixture_segment":
			if s.config.LiveSTTAdapter != "fake" || strings.TrimSpace(message.Text) == "" {
				continue
			}
			if err := s.appendAssistantVoiceText(
				request.Context(),
				state,
				"fixture-"+uuid.NewString(),
				message.Text,
				message.StartMS,
				max(message.EndMS, message.StartMS+1),
				message.Confidence,
			); err != nil {
				return
			}
		case "edit_transcript":
			if err := s.editAssistantVoiceTranscript(
				request.Context(),
				state,
				message.Text,
			); err != nil {
				_ = writer.write(request.Context(), map[string]any{
					"type": "edit_rejected", "reason": "invalid_transcript",
				})
				continue
			}
			_ = writer.write(request.Context(), map[string]any{
				"type": "transcript_updated", "transcript": state.transcript(),
			})
			if !state.awaitingConfirmation() {
				state.resetSilenceTimer(request.Context())
			}
		case "confirm_execute":
			state.stopTimer()
			turnID, err := s.confirmAssistantVoiceTurn(request.Context(), state)
			if err != nil {
				_ = writer.write(request.Context(), map[string]any{
					"type": "execution_rejected", "reason": "invalid_turn",
				})
				continue
			}
			started := s.startAssistantExecution(user.ID, run.ID, turnID)
			_ = writer.write(request.Context(), map[string]any{
				"type": "turn_queued", "turnId": turnID, "started": started,
			})
			if started {
				return
			}
		case "continue_collecting":
			state.stopTimer()
			if err := s.continueAssistantVoiceTurn(request.Context(), state); err != nil {
				_ = writer.write(request.Context(), map[string]any{
					"type": "continue_rejected", "reason": "invalid_turn",
				})
				continue
			}
			_ = writer.write(request.Context(), map[string]any{
				"type": "confirmation_dismissed", "transcript": state.transcript(),
			})
		case "flush_live":
			if provider != nil {
				flushContext, cancel := context.WithTimeout(request.Context(), 4*time.Second)
				if err := provider.Finalize(flushContext); err == nil {
					select {
					case <-state.finalized:
					case <-flushContext.Done():
					}
				}
				cancel()
			}
			_ = writer.write(request.Context(), map[string]any{
				"type": "live_flushed", "transcript": state.transcript(),
			})
		case "discard":
			state.stopTimer()
			state.mu.Lock()
			turn := state.turn
			state.turn = nil
			state.segments = nil
			state.mu.Unlock()
			if turn != nil {
				if err := s.store.DeleteAssistantDraftTurn(
					request.Context(),
					state.ownerID,
					state.runID,
					turn.ID,
				); err != nil {
					_ = writer.write(request.Context(), map[string]any{
						"type": "discard_rejected", "reason": "draft_unavailable",
					})
					continue
				}
			}
			_ = writer.write(request.Context(), map[string]any{
				"type": "draft_discarded",
			})
			return
		case "ping":
			_ = writer.write(request.Context(), map[string]string{"type": "pong"})
		}
	}
}

func (s *Server) forwardAssistantVoiceEvents(
	ctx context.Context,
	state *assistantVoiceState,
	provider stt.LiveSession,
) {
	for {
		event, err := provider.ReadEvent(ctx)
		if err != nil {
			if ctx.Err() == nil {
				_ = state.writer.write(context.WithoutCancel(ctx), map[string]any{
					"type": "stream_degraded", "reason": "stt_unavailable",
				})
			}
			return
		}
		switch event.Kind {
		case "partial":
			state.resetSilenceTimer(ctx)
			_ = state.writer.write(ctx, map[string]any{
				"type": "partial", "providerTurnId": event.ProviderTurnID,
				"text": event.Text, "startMs": event.StartMS, "endMs": event.EndMS,
			})
		case "final":
			if err := s.appendAssistantVoiceText(
				ctx,
				state,
				event.ProviderTurnID,
				event.Text,
				event.StartMS,
				max(event.EndMS, event.StartMS+1),
				event.Confidence,
			); err != nil {
				_ = state.writer.write(ctx, map[string]any{
					"type": "stream_degraded", "reason": "persistence_unavailable",
				})
				return
			}
		case "speech_started":
			state.resetSilenceTimer(ctx)
			_ = state.writer.write(ctx, map[string]any{"type": event.Kind})
		case "finalized":
			select {
			case state.finalized <- struct{}{}:
			default:
			}
			_ = state.writer.write(ctx, map[string]any{"type": event.Kind})
		case "speech_stopped":
			_ = state.writer.write(ctx, map[string]any{"type": event.Kind})
		}
	}
}

func (s *Server) appendAssistantVoiceText(
	ctx context.Context,
	state *assistantVoiceState,
	providerTurnID string,
	text string,
	startMS int64,
	endMS int64,
	confidence *float64,
) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	switch parseAssistantVoiceCommand(text) {
	case assistantVoiceCommandExecute:
		state.stopTimer()
		turnID, err := s.confirmAssistantVoiceTurn(ctx, state)
		if err != nil {
			_ = state.writer.write(ctx, map[string]any{
				"type": "command_rejected", "command": "execute", "reason": "empty_turn",
			})
			return nil
		}
		started := s.startAssistantExecution(state.ownerID, state.runID, turnID)
		_ = state.writer.write(ctx, map[string]any{
			"type": "command_recognized", "command": "execute",
			"label": "Выполнить поручение",
		})
		_ = state.writer.write(ctx, map[string]any{
			"type": "turn_queued", "turnId": turnID, "started": started,
		})
		return nil
	case assistantVoiceCommandContinue:
		state.stopTimer()
		if err := s.continueAssistantVoiceTurn(ctx, state); err != nil {
			_ = state.writer.write(ctx, map[string]any{
				"type": "command_rejected", "command": "continue", "reason": "empty_turn",
			})
			return nil
		}
		_ = state.writer.write(ctx, map[string]any{
			"type": "command_recognized", "command": "continue",
			"label": "Продолжить говорить",
		})
		_ = state.writer.write(ctx, map[string]any{
			"type": "confirmation_dismissed", "transcript": state.transcript(),
		})
		return nil
	}
	state.mu.Lock()
	state.segments = append(state.segments, text)
	combined := strings.TrimSpace(strings.Join(state.segments, " "))
	if state.turn == nil {
		created, err := s.store.CreateAssistantTurnWithSelection(
			ctx,
			state.ownerID,
			state.runID,
			assistant.InputVoice,
			combined,
			assistant.TurnCollecting,
			state.selection,
		)
		if err != nil {
			state.mu.Unlock()
			return err
		}
		state.turn = &created
	} else {
		state.turn.Transcript = combined
		state.turn.Instruction = combined
		state.turn.State = assistant.TurnCollecting
		if err := s.store.UpdateAssistantTurn(ctx, *state.turn); err != nil {
			state.mu.Unlock()
			return err
		}
	}
	turn := *state.turn
	state.mu.Unlock()
	segmentID, sequence, err := s.store.AppendAssistantVoiceSegment(
		ctx,
		state.ownerID,
		state.runID,
		turn.ID,
		providerTurnID,
		text,
		startMS,
		endMS,
		confidence,
	)
	if err != nil {
		return err
	}
	_ = state.writer.write(ctx, map[string]any{
		"type": "segment_final", "segmentId": segmentID, "sequence": sequence,
		"turnId": turn.ID, "text": text, "transcript": combined,
		"startMs": startMS, "endMs": endMS, "confidence": confidence,
	})
	state.resetSilenceTimer(ctx)
	return nil
}

type assistantVoiceCommand string

const (
	assistantVoiceCommandNone     assistantVoiceCommand = ""
	assistantVoiceCommandExecute  assistantVoiceCommand = "execute"
	assistantVoiceCommandContinue assistantVoiceCommand = "continue"
)

func parseAssistantVoiceCommand(text string) assistantVoiceCommand {
	normalized := strings.Join(strings.Fields(strings.Map(func(value rune) rune {
		if unicode.IsPunct(value) || unicode.IsSymbol(value) {
			return ' '
		}
		return unicode.ToLower(value)
	}, text)), " ")
	switch normalized {
	case "команда выполнить поручение":
		return assistantVoiceCommandExecute
	case "команда продолжить":
		return assistantVoiceCommandContinue
	default:
		return assistantVoiceCommandNone
	}
}

func (s *Server) editAssistantVoiceTranscript(
	ctx context.Context,
	state *assistantVoiceState,
	text string,
) error {
	text = strings.TrimSpace(text)
	if text == "" || len([]rune(text)) > assistant.MaxInstructionChars {
		return assistant.ErrInvalidTurn
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.turn == nil {
		created, err := s.store.CreateAssistantTurnWithSelection(
			ctx,
			state.ownerID,
			state.runID,
			assistant.InputVoice,
			text,
			assistant.TurnCollecting,
			state.selection,
		)
		if err != nil {
			return err
		}
		state.segments = []string{text}
		state.turn = &created
		return nil
	}
	state.segments = []string{text}
	state.turn.Transcript = text
	state.turn.Instruction = text
	if state.turn.State != assistant.TurnAwaitingConfirmation {
		state.turn.State = assistant.TurnCollecting
	}
	return s.store.UpdateAssistantTurn(ctx, *state.turn)
}

func (s *Server) setAssistantVoiceSelection(
	ctx context.Context,
	state *assistantVoiceState,
	selection *assistant.SelectionAttachment,
) error {
	if err := s.validateAssistantSelection(
		ctx, state.ownerID, state.runID, selection,
	); err != nil {
		return err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	state.selection = selection
	if state.turn != nil && (state.turn.State == assistant.TurnCollecting ||
		state.turn.State == assistant.TurnAwaitingConfirmation) {
		state.turn.SelectionAttachment = selection
		return s.store.UpdateAssistantTurn(ctx, *state.turn)
	}
	return nil
}

func (v *assistantVoiceState) awaitingConfirmation() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.turn != nil && v.turn.State == assistant.TurnAwaitingConfirmation
}

func (s *Server) continueAssistantVoiceTurn(
	ctx context.Context,
	state *assistantVoiceState,
) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.turn == nil {
		return errors.New("voice turn was not started")
	}
	state.turn.State = assistant.TurnCollecting
	return s.store.UpdateAssistantTurn(ctx, *state.turn)
}

func (s *Server) confirmAssistantVoiceTurn(
	ctx context.Context,
	state *assistantVoiceState,
) (string, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.turn == nil {
		return "", errors.New("voice turn was not started")
	}
	state.turn.State = assistant.TurnQueued
	if err := state.turn.ValidateInstruction(); err != nil {
		return "", err
	}
	if err := s.store.UpdateAssistantTurn(ctx, *state.turn); err != nil {
		return "", err
	}
	return state.turn.ID, nil
}

// Kept local so this file remains testable without reaching into chi internals.
var chiURLParam = func(request *http.Request, name string) string {
	return chi.URLParam(request, name)
}
