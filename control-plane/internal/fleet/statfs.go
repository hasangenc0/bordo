package fleet

import "syscall"

// DiskOf measures the filesystem that holds path. Inside a container the root filesystem is an
// overlay whose size is the host's, so statting "/" reports the machine's disk.
func DiskOf(path string) *DiskUsage {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return nil
	}
	bsize := int64(st.Bsize)
	total := int64(st.Blocks) * bsize
	free := int64(st.Bavail) * bsize
	d := NewDiskUsage(total, free)
	return d
}
