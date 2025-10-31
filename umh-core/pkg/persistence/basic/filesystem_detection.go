package basic

import (
	"strings"
)

// IsNetworkFSType checks if a filesystem type name indicates a network filesystem.
//
// DESIGN DECISION: Case-insensitive substring matching
// WHY: Filesystem type names vary across platforms and versions.
// Examples: "nfs", "nfs4", "NFSv4", "cifs", "smbfs", "webdav"
//
// NETWORK FILESYSTEM TYPES DETECTED:
//   - NFS: Network File System (all versions)
//   - CIFS/SMB: Windows file sharing protocols
//   - WebDAV: HTTP-based file sharing
//
// LIMITATION: Does not detect all network filesystems
// (e.g., FUSE-based network mounts with generic type names)
// Conservative approach: only block known-dangerous types.
func IsNetworkFSType(fsType string) bool {
	networkTypes := []string{"nfs", "cifs", "smb", "webdav"}
	fsTypeLower := strings.ToLower(fsType)

	for _, nt := range networkTypes {
		if strings.Contains(fsTypeLower, nt) {
			return true
		}
	}

	return false
}
