package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// sidecarName is the file extract drops into its output directory to record
// the source pak's mount point, so build can reproduce it without the user
// having to remember and retype it.
const sidecarName = ".unrealpak.json"

type sidecar struct {
	MountPoint string `json:"mountPoint"`
}

func writeSidecar(dir, mountPoint string) error {
	data, err := json.MarshalIndent(sidecar{MountPoint: mountPoint}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, sidecarName), append(data, '\n'), 0o644)
}

// readSidecar returns the mount point recorded in dir. A missing sidecar is
// reported as such so build can tell "no recorded mount" apart from "the
// sidecar is corrupt" and ask the user for --mount in the first case only.
func readSidecar(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, sidecarName))
	if err != nil {
		return "", err
	}
	var s sidecar
	if err := json.Unmarshal(data, &s); err != nil {
		return "", fmt.Errorf("parsing %s: %w", sidecarName, err)
	}
	if s.MountPoint == "" {
		return "", fmt.Errorf("%s records no mountPoint", sidecarName)
	}
	return s.MountPoint, nil
}
