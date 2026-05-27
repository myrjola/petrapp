// Package errorrecorder buffers recent slog records keyed by session_hash
// (or trace_id) and, when an Error-level record is observed, dumps the
// matching session's records to a per-occurrence file under a configured
// logs directory. The on-disk format mirrors the wrapped slog.Handler so
// dump files are directly comparable with the live log stream.
package errorrecorder
