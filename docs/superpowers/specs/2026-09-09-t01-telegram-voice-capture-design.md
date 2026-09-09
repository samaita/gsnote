# T01 Design — Telegram Voice Capture + Async Verbatim Transcription

## Decision

Implement a single-user Telegram voice pipeline backed by SQLite. The handler downloads and durably stores original OGG audio, records note metadata, queues a transcription job, and immediately acknowledges capture. One background worker claims jobs atomically and sends the resulting `.txt` file when complete.

Transcription is replaceable behind `Transcriber`. The initial adapter is local `whisper.cpp`, configured by environment variables and invoked without GPU assumptions. No paid transcription API is used.

## Components and flow

- `telegram/handler`: allowlist enforcement, voice update routing, immediate ACK, completion/failure messaging.
- `notes/storage`: audio and transcript directories under `DATA_DIR`; deterministic `VN-YYYYMMDD-HHMMSS-XXXX` IDs and filenames.
- `jobs/repository`: SQLite schema, durable job states, atomic claim, stale `TRANSCRIBING` recovery.
- `transcription`: `Transcriber` interface and local whisper.cpp process adapter.
- `worker`: exactly one polling worker; claim oldest `QUEUED`, transcribe, write raw text, update state, notify Telegram.

Normal state flow: `RECEIVED` → `STORED` → `QUEUED` → `TRANSCRIBING` → `DONE`. Transcription errors produce `FAILED` while retaining audio and error details. Startup recovery requeues stale jobs.

## Configuration and deployment

Required: `TELEGRAM_BOT_TOKEN`, `ALLOWED_TELEGRAM_USER_ID`, `DATA_DIR`. Optional: `TZ`, `LOG_LEVEL`, `TRANSCRIBER_MODEL`, `TRANSCRIBER_THREADS`, `TRANSCRIBER_BINARY`, `TRANSCRIBER_LANGUAGE`, and stale-job timeout. Docker uses a persistent `/data` volume, two CPU-oriented worker settings, and no GPU.

## Verification

Unit tests cover ID/filename generation, SQLite transitions and atomic claiming, stale recovery, audio persistence, verbatim transcript writing, allowlist behavior, async worker completion/failure, and notifier attachment naming. `go test ./...`, `go build ./...`, and deployment config syntax/build checks are required.

## Scope exclusions

No summaries, rewriting, translation, tagging, search, embeddings, web UI, mobile app, multi-user support, realtime transcription, diarization, or extra workers.
