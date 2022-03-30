package main

type ProcessValue struct{}

// Contains timestamp_ms and 1 other key, which is a float64
type processValue map[string]interface{}

func (c ProcessValue) ProcessMessages(msg ParsedMessage) (err error, putback bool) {
	//TODO do this like count !
	return nil, true
}
