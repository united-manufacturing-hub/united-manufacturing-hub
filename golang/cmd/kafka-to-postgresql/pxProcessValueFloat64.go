package main

type ProcessValueFloat64 struct{}

// Contains timestamp_ms and 1 other key, which is a float64
type processValueFloat64 map[string]interface{}

func (c ProcessValueFloat64) ProcessMessages(msg ParsedMessage) (err error, putback bool) {
	//TODO do this like count !
	return nil, true
}
