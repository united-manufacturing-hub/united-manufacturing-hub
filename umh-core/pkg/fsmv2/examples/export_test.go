package examples

import "time"

// SetWaitForTimeoutForTest overrides how long a single WaitFor may take
// before it fails the run, and returns a func restoring the default. It is
// exported only for the package's tests.
func SetWaitForTimeoutForTest(d time.Duration) (restore func()) {
	prev := waitForTimeout
	waitForTimeout = d

	return func() {
		waitForTimeout = prev
	}
}
