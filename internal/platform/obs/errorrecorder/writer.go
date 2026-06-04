package errorrecorder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// maxCollisionRetries bounds how many "-N" suffixes we try when the
// same-second filename already exists. Beyond this we surface the error.
const maxCollisionRetries = 9

// keyPrefixLen is the number of leading characters of the session key
// used in the dump filename (keeps the name short while still being
// uniquely identifiable in practice).
const keyPrefixLen = 8

// writeDump serialises records to <root>/<YYYY>/<MM>/<DD>/<ts>-<key8>.jsonl
// using a fresh slog.JSONHandler so the on-disk format matches the live
// stdout stream. ts is formatted as UTC YYYYMMDDTHHMMSSZ. key8 is the first
// 8 chars of key (or fewer if key is shorter). Returns the final file path.
func writeDump(
	root string,
	key string,
	ts time.Time,
	records []bufferedRecord,
	opts *slog.HandlerOptions,
) (string, error) {
	if len(records) == 0 {
		return "", errors.New("writeDump: no records")
	}
	ts = ts.UTC()
	dayDir := filepath.Join(root,
		fmt.Sprintf("%04d", ts.Year()),
		fmt.Sprintf("%02d", ts.Month()),
		fmt.Sprintf("%02d", ts.Day()),
	)
	if err := os.MkdirAll(dayDir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	key8 := key
	if len(key8) > keyPrefixLen {
		key8 = key8[:keyPrefixLen]
	}
	base := fmt.Sprintf("%s-%s", ts.Format("20060102T150405Z"), key8)

	for attempt := 0; attempt <= maxCollisionRetries; attempt++ {
		name := base + ".jsonl"
		if attempt > 0 {
			name = fmt.Sprintf("%s-%d.jsonl", base, attempt)
		}
		path := filepath.Join(dayDir, name)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", fmt.Errorf("open: %w", err)
		}
		writeErr := replayRecords(f, records, opts)
		closeErr := f.Close()
		if writeErr != nil {
			return "", writeErr
		}
		if closeErr != nil {
			return "", fmt.Errorf("close: %w", closeErr)
		}
		return path, nil
	}
	return "", fmt.Errorf("dump filename collision retries exhausted under %s", dayDir)
}

// replayRecords pipes each bufferedRecord through a fresh slog.JSONHandler
// writing into w. The handler is configured with opts so dump output matches
// the live JSONHandler's encoding.
func replayRecords(w *os.File, records []bufferedRecord, opts *slog.HandlerOptions) error {
	h := slog.NewJSONHandler(w, opts)
	ctx := context.Background()
	for _, br := range records {
		if err := h.Handle(ctx, br.record); err != nil {
			return fmt.Errorf("handle: %w", err)
		}
	}
	return nil
}
