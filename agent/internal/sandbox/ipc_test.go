package sandbox

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// Tables as the kernel prints them in /proc/sysvipc (trailing time columns trimmed).
const (
	shmTable = `       key      shmid perms                  size  cpid  lpid nattch   uid   gid  cuid  cgid      atime
         0         32  1600               1048576   812   812      0 10000 10000 10000 10000          0
         0         33  1600               1048576   813   813      0 10001 10000 10001 10000          0
`
	msgTable = `       key      msqid perms      cbytes       qnum lspid lrpid   uid   gid  cuid  cgid      stime
1234             7   666           0          0     0     0     0     0 10000 10000          0
`
	semTable = `       key      semid perms      nsems   uid   gid  cuid  cgid      otime
         0          3   600          1 10000 10000 10000 10000          0
         0          4   600          1     0     0     0     0          0
`
)

func TestIPCIDsOwnedBy(t *testing.T) {
	if got := ipcIDsOwnedBy(shmTable, 10000); !reflect.DeepEqual(got, []string{"32"}) {
		t.Errorf("shm = %v", got)
	}
	// Created by the slot user but handed to root: still the run's, still removed.
	if got := ipcIDsOwnedBy(msgTable, 10000); !reflect.DeepEqual(got, []string{"7"}) {
		t.Errorf("msg = %v", got)
	}
	if got := ipcIDsOwnedBy(semTable, 10000); !reflect.DeepEqual(got, []string{"3"}) {
		t.Errorf("sem = %v", got)
	}
	if got := ipcIDsOwnedBy("       key      shmid perms   uid\n", 10000); got != nil {
		t.Errorf("empty table = %v", got)
	}
}

func TestSysvIPCOwnedByReadsAllTables(t *testing.T) {
	dir := t.TempDir()
	for name, table := range map[string]string{"shm": shmTable, "msg": msgTable, "sem": semTable} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(table), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := sysvIPCOwnedBy(dir, 10000)
	sort.Strings(got)
	if want := []string{"m:32", "q:7", "s:3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("objects = %v, want %v", got, want)
	}
	if got := sysvIPCOwnedBy(filepath.Join(dir, "missing"), 10000); len(got) != 0 {
		t.Errorf("no /proc/sysvipc must mean nothing to remove, got %v", got)
	}
}

func TestRunIPCRMRejectsBadArguments(t *testing.T) {
	if RunIPCRM([]string{"x", "m:notanumber"}) == 0 {
		t.Error("malformed objects must fail")
	}
	if RunIPCRM(nil) != 0 {
		t.Error("nothing to remove is success")
	}
}
