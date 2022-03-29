package main

type MessageProcessor interface {
	ProcessMessage(customerID string, location string, assetID string, payload []byte) (err error)
}
