//go:build darwin

package basic

import (
	"fmt"
	"syscall"
)

// IsNetworkFilesystem detects if the given path resides on a network filesystem.
//
// PLATFORM: macOS (Darwin)
// METHOD: syscall.Statfs() with Fstypename field inspection
//
// DESIGN DECISION: Use syscall.Statfs instead of parsing df/mount output
// WHY: syscall is portable, fast, doesn't depend on external tools
//
// macOS-SPECIFIC: Statfs_t contains Fstypename[16]byte field with FS type
// Examples: "apfs", "hfs", "nfs", "smbfs", "webdav"
//
// Returns:
//   - isNetwork: true if filesystem is network-based (NFS/CIFS/SMB/WebDAV)
//   - fsType: human-readable filesystem type name
//   - err: error if syscall fails or path doesn't exist
func IsNetworkFilesystem(path string) (bool, string, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return false, "", fmt.Errorf("failed to stat filesystem at %s: %w", path, err)
	}

	var fsTypeBytes [16]byte
	for i, v := range stat.Fstypename {
		fsTypeBytes[i] = byte(v)
	}

	fsType := string(fsTypeBytes[:clen(fsTypeBytes[:])])

	return IsNetworkFSType(fsType), fsType, nil
}

func clen(n []byte) int {
	for i := range n {
		if n[i] == 0 {
			return i
		}
	}

	return len(n)
}
