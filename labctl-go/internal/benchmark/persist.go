package benchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// PersistCapture writes a complete capture to a temporary file, fsyncs it, and
// publishes it with an atomic no-overwrite hard link. Existing evidence is
// never silently replaced.
func PersistCapture(path string, capture Capture) error {
	payload, err := json.MarshalIndent(capture, "", "  ")
	if err != nil {
		return fmt.Errorf("encode capture: %w", err)
	}
	payload = append(payload, '\n')

	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := requireAbsent(path); err != nil {
		return err
	}

	temporary, err := os.CreateTemp(directory, ".paraflow-capture-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary capture: %w", err)
	}
	temporaryPath := temporary.Name()
	published := false
	defer func() {
		_ = temporary.Close()
		if !published {
			_ = os.Remove(temporaryPath)
		}
	}()

	if err := temporary.Chmod(0o644); err != nil {
		return fmt.Errorf("set temporary capture permissions: %w", err)
	}
	if _, err := temporary.Write(payload); err != nil {
		return fmt.Errorf("write temporary capture: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary capture: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary capture: %w", err)
	}

	if err := os.Link(temporaryPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("output path %q already exists", path)
		}
		return fmt.Errorf("publish capture atomically: %w", err)
	}
	published = true
	if err := os.Remove(temporaryPath); err != nil {
		return fmt.Errorf("remove temporary capture link: %w", err)
	}

	directoryHandle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open output directory for sync: %w", err)
	}
	defer func() {
		_ = directoryHandle.Close()
	}()
	if err := directoryHandle.Sync(); err != nil {
		return fmt.Errorf("sync output directory: %w", err)
	}
	return nil
}
