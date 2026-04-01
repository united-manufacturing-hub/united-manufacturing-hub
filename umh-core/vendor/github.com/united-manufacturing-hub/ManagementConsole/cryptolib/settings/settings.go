package settings

import "time"

// Argon2 parameters (https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)
/*
m=47104 (46 MiB), t=1, p=1 (Do not use with Argon2i)
m=19456 (19 MiB), t=2, p=1 (Do not use with Argon2i)
m=12288 (12 MiB), t=3, p=1
m=9216 (9 MiB), t=4, p=1
m=7168 (7 MiB), t=5, p=1
*/

// For our paramaters, we choose high memory and low parallelism, as WASM is single-threaded
const (
	// Time cost parameter
	ARGON2ID_TIME = 1

	// Memory cost parameter (in KB)
	ARGON2ID_MEMORY = 46 * 1024 // 46 MiB

	// Parallelism parameter
	ARGON2ID_THREADS = 1

	// Key length (in bytes)
	ARGON2ID_KEY_LENGTH = 32

	// Server-side pepper (this should be configured via environment variable in production)
	ARGON2ID_STATIC_PEPPER = "71cd045903aac2f546062e6741f6538682b25a6fc3e1c204275c320a95dc1de1"
)

// TLS_READ_TIMEOUT and WRITE_TIMEOUT is the maximum time we expect a message
// to take from the frontend to the instance
// or from the instance to the frontend
// Note: Increasing this, can lead to the frontend or instance stalling
// We use 2x, to account for the initial handshake
const TLS_READ_TIMEOUT = 45 * 2 * time.Second
const TLS_WRITE_TIMEOUT = 45 * 2 * time.Second
