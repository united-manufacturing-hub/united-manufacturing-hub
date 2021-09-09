package main

type QueueHandler interface {
	// EnqueueMQTT parses an MQTT message and then enqueues it
	EnqueueMQTT(customerID string, location string, assetID string, payload []byte)
	// enqueue is in internal method to enqueue the message to its internal queue
	enqueue(bytes []byte, priority uint8)
	// process should loop until shutdown and process the messages in its queue
	process()
	// Shutdown shuts the process goroutine down and then closes the queue
	Shutdown() (err error)
	// reportLength prints the current queue length
	reportLength()
}
