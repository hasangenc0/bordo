package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// System V IPC objects (shared memory, message queues, semaphore sets) are not files and do not die
// with their processes: a segment a test creates stays in memory until someone removes it. Teardown
// removes the slot user's objects. Root lacks CAP_IPC_OWNER, so the removal runs as that user, by
// re-running this binary with IPCRMCommand.

// IPCRMCommand is the hidden argument that makes the sandbox binary remove IPC objects and exit.
const IPCRMCommand = "__ipcrm"

// ipcKinds maps the /proc/sysvipc file to the tag used on the command line.
var ipcKinds = map[string]string{"shm": "m", "msg": "q", "sem": "s"}

// sysvIPCOwnedBy lists the slot user's IPC objects as "m:<id>", "q:<id>" or "s:<id>".
func sysvIPCOwnedBy(procDir string, uid int) []string {
	var objs []string
	for file, tag := range ipcKinds {
		raw, err := os.ReadFile(filepath.Join(procDir, file))
		if err != nil {
			continue
		}
		for _, id := range ipcIDsOwnedBy(string(raw), uid) {
			objs = append(objs, tag+":"+id)
		}
	}
	return objs
}

// ipcIDsOwnedBy parses one /proc/sysvipc table and returns the ids of objects whose owner or creator
// is uid. Columns differ per table, so they are found by header name.
func ipcIDsOwnedBy(table string, uid int) []string {
	lines := strings.Split(strings.TrimSpace(table), "\n")
	if len(lines) < 2 {
		return nil
	}
	idCol, uidCol, cuidCol := -1, -1, -1
	for i, h := range strings.Fields(lines[0]) {
		switch h {
		case "shmid", "msqid", "semid":
			idCol = i
		case "uid":
			uidCol = i
		case "cuid":
			cuidCol = i
		}
	}
	if idCol < 0 || uidCol < 0 {
		return nil
	}
	want := strconv.Itoa(uid)
	var ids []string
	for _, line := range lines[1:] {
		f := strings.Fields(line)
		if len(f) <= idCol || len(f) <= uidCol {
			continue
		}
		if f[uidCol] == want || (cuidCol >= 0 && cuidCol < len(f) && f[cuidCol] == want) {
			ids = append(ids, f[idCol])
		}
	}
	return ids
}

// RunIPCRM removes the IPC objects named in args ("m:<id>", "q:<id>", "s:<id>") and returns an exit
// code. It is what the binary does when started with IPCRMCommand.
func RunIPCRM(args []string) int {
	failed := 0
	for _, a := range args {
		tag, idStr, ok := strings.Cut(a, ":")
		id, err := strconv.Atoi(idStr)
		if !ok || err != nil {
			failed++
			continue
		}
		if err := removeIPC(tag, id); err != nil {
			fmt.Fprintf(os.Stderr, "ipcrm %s: %v\n", a, err)
			failed++
		}
	}
	if failed > 0 {
		return 1
	}
	return 0
}
