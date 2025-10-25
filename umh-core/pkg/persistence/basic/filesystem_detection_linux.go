//go:build linux

package basic

import (
	"fmt"
	"syscall"
)

// Linux filesystem type magic numbers
// Source: man 2 statfs, linux/magic.h
const (
	nfsMagic     = 0x6969     // NFS
	nfs4Magic    = 0x6e667332 // NFS v4
	cifsMagic    = 0xff534d42 // CIFS/SMB
	smbMagic     = 0x517b      // SMB
	fuseblkMagic = 0x65735546 // FUSE (may be network)
)

// IsNetworkFilesystem detects if the given path resides on a network filesystem.
//
// PLATFORM: Linux
// METHOD: syscall.Statfs() with Type field inspection (filesystem magic number)
//
// DESIGN DECISION: Use syscall.Statfs instead of parsing /proc/mounts
// WHY: syscall is portable, fast, doesn't depend on proc filesystem
//
// LINUX-SPECIFIC: Statfs_t contains Type field with filesystem magic number
// Examples: 0x6969 (NFS), 0xff534d42 (CIFS), 0xef53 (ext4)
//
// LIMITATION: FUSE-based network mounts may not be detected
// We check for FUSE magic but can't distinguish network vs local FUSE mounts
//
// Returns:
//   - isNetwork: true if filesystem is network-based (NFS/CIFS/SMB)
//   - fsType: human-readable filesystem type name
//   - err: error if syscall fails or path doesn't exist
func IsNetworkFilesystem(path string) (bool, string, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return false, "", fmt.Errorf("failed to stat filesystem at %s: %w", path, err)
	}

	// Map filesystem Type (magic number) to human-readable name
	fsType, isNetwork := getFSTypeFromMagic(stat.Type)

	return isNetwork, fsType, nil
}

// getFSTypeFromMagic converts Linux filesystem magic number to type name
func getFSTypeFromMagic(magic int64) (string, bool) {
	switch magic {
	case nfsMagic:
		return "nfs", true
	case nfs4Magic:
		return "nfs4", true
	case cifsMagic:
		return "cifs", true
	case smbMagic:
		return "smb", true
	case fuseblkMagic:
		// FUSE can be local or network - conservative: not network
		return "fuse", false
	case 0xef53:
		return "ext4", false
	case 0x58465342:
		return "xfs", false
	case 0x9123683e:
		return "btrfs", false
	case 0x01021994:
		return "tmpfs", false
	default:
		// Unknown filesystem type - return hex representation
		return fmt.Sprintf("unknown(0x%x)", magic), false
	}
}
