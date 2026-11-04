//go:build linux && (amd64 || arm64)

package sandbox

import (
	"fmt"
	"syscall"
)

const ipcRMID = 0

func removeIPC(tag string, id int) error {
	var trap uintptr
	switch tag {
	case "m":
		trap = syscall.SYS_SHMCTL
	case "q":
		trap = syscall.SYS_MSGCTL
	case "s":
		trap = syscall.SYS_SEMCTL
	default:
		return fmt.Errorf("unknown IPC kind %q", tag)
	}
	// shmctl/msgctl(id, IPC_RMID, NULL) and semctl(id, 0, IPC_RMID).
	a2, a3 := uintptr(ipcRMID), uintptr(0)
	if tag == "s" {
		a2, a3 = 0, ipcRMID
	}
	if _, _, errno := syscall.Syscall(trap, uintptr(id), a2, a3); errno != 0 {
		return errno
	}
	return nil
}
