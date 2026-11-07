# ADR-001: Capability-based speech recognition

Status: accepted; implemented in the frozen Go/web edition.

## Context

Live feedback, durable audio capture and a high-quality final transcript have
different failure and latency characteristics. A single nominally
OpenAI-compatible file endpoint does not provide a continuous realtime stream,
word timings or consistent finalization semantics.

## Decision

Use separate live and file-recognition capabilities behind server-side adapters.
Deepgram provides the native continuous live stream. OpenRouter provides explicit
file re-transcription and fallback finalization when live completion is unavailable.
Keep deterministic fake adapters behind an explicit non-production test gate.

The browser records local audio independently of recognition. It sends mono PCM16
transport frames to the Go WebSocket gateway, which keeps the provider connection
open for the session. Normalize partial text, final segments, punctuation, word
intervals, confidence, errors and end-of-stream acknowledgement.

Stable segments remain distinct from replaceable partial text. Neighboring final
segments form a continuous prose run unless an explicit structural command creates
a paragraph boundary. Successful live finalization becomes the first immutable
source transcript and draft revision; it does not trigger a second paid provider
pass automatically.

## Merge and recovery rules

- Original audio and source transcript artifacts are immutable.
- A late quality pass creates a separate suggestion; manual drafts/manuscripts win.
- Dictionary hints affect future recognition and explicit re-transcription, not
  already accepted text.
- Network/provider failure does not stop local recording. Pending chunks retry
  idempotently; finalization jobs survive page/process restarts.
- Advertise actual capabilities. Missing credentials disable recognition rather
  than displaying synthetic fixture output to real users.

## Alternatives and consequences

A single file-only Whisper endpoint is simpler but cannot guarantee true live
feedback. Provider-hosted recognition requires processing disclosure and incurs
external costs. The adapter boundary allows future implementations without
changing source/draft/revision semantics; no such implementation is promised.

Evaluate quality with consented or synthetic Russian/English speech, proper names,
long sessions, reconnects, silence, punctuation and timestamps. Measure latency,
word/name errors and recovery behavior separately. See `internal/stt` and the
opt-in real-provider tests; provider documentation and pricing can change.
