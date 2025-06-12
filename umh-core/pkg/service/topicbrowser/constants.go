package topicbrowser

const (
	// BLOCK_START_MARKER marks the begin of a new data/general block inside the logs.
	BLOCK_START_MARKER = "STARTSTARTSTART"
	// DATA_END_MARKER marks the end of a data block inside the logs.
	DATA_END_MARKER = "ENDDATAENDDATAENDDATA"
	// between DATA_END_MARKER AND BLOCK_END_MARKER sits the timestamp.

	// BLOCK_END_MARKER marks the end of a general block inside the logs.
	BLOCK_END_MARKER = "ENDENDEND"
)
