//go:build !(linux && (amd64 || arm64))

package sandbox

import "errors"

func removeIPC(string, int) error {
	return errors.New("System V IPC removal is not supported on this platform")
}
