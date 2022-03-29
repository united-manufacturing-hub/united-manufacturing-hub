package main

type ProcessValueString struct{}

// Contains timestamp_ms and 1 other key, which is a string
type processValueString map[string]interface{}

func (c ProcessValueString) ProcessMessages(msg ParsedMessage, pid int) (err error, putback bool) {
	//TODO do this like count !
	return nil, true
}
