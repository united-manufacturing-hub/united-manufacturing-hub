package maptostruct

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/tools/safejson"
)

func MapToStruct(payloadMap map[string]interface{}, dest interface{}) error {
	payloadBytes, err := safejson.Marshal(payloadMap)
	if err != nil {
		return err
	}
	err = safejson.Unmarshal(payloadBytes, dest)
	if err != nil {
		return fmt.Errorf("failed to map into %T: %w", dest, err)
	}
	return nil
}
