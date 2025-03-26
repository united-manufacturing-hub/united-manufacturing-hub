package safejson

import (
	"encoding/base64"
	jsonstd "encoding/json"
	"fmt"
	"reflect"

	"github.com/goccy/go-json"
	"go.uber.org/zap"
)

func Unmarshal(val []byte, decoded any) (err error) {
	valuePtr := reflect.ValueOf(decoded)
	if valuePtr.Kind() != reflect.Ptr || valuePtr.IsNil() || !valuePtr.IsValid() {
		return fmt.Errorf("decoded must be a non-nil pointer")
	}

	// Attempt decoding with goccy, fallback to stdlib on panic
	defer func() {
		if r := recover(); r != nil {
			b64payload := base64.StdEncoding.EncodeToString(val)
			zap.S().Warnf("goccy failed to decode, attempting to use stdlib, error: %v (Payload: %s)", r, b64payload)
			// Validate that valuePtr is still a valid pointer before attempting further actions
			if !valuePtr.IsNil() && valuePtr.IsValid() && !valuePtr.Elem().IsNil() && valuePtr.Elem().IsValid() {
				temp := reflect.New(valuePtr.Elem().Type()).Interface()
				err = jsonstd.Unmarshal(val, &temp)
				if err == nil {
					valuePtr.Elem().Set(reflect.ValueOf(temp).Elem())
				}
			} else {
				err = fmt.Errorf("decoded type became invalid: %v", r)
			}
		}
	}()

	if valuePtr.Elem().Kind() != reflect.Struct {
		// Try stdlib unmarshal
		err = jsonstd.Unmarshal(val, decoded)
		return err
	}

	temp := reflect.New(valuePtr.Elem().Type()).Interface()
	err = json.Unmarshal(val, &temp)
	if err == nil {
		valuePtr.Elem().Set(reflect.ValueOf(temp).Elem())
	}

	return err
}

func Marshal(val any) (encoded []byte, err error) {
	// This will attempt encoding with goccy, if goccy panics it will attempt to use stdlib
	defer func() {
		if r := recover(); r != nil {
			zap.S().Warnf("goccy failed to encode, attempting to use stdlib, error: %v", r)
			encoded, err = jsonstd.Marshal(val)
		}
	}()
	encoded, err = json.Marshal(val)
	return encoded, err
}

func MarshalIndent(val any, prefix, indent string) (encoded []byte, err error) {
	// This will attempt encoding with goccy, if goccy panics it will attempt to use stdlib
	defer func() {
		if r := recover(); r != nil {
			zap.S().Warnf("goccy failed to encode, attempting to use stdlib, error: %v", r)
			encoded, err = jsonstd.MarshalIndent(val, prefix, indent)
		}
	}()
	encoded, err = json.MarshalIndent(val, prefix, indent)
	return encoded, err
}

// MustUnmarshal is a helper function that unmarshals a JSON string into a struct.
// It panics if the unmarshal operation fails.
func MustUnmarshal(val []byte, decoded any) {
	err := Unmarshal(val, decoded)
	if err != nil {
		panic(err)
	}
}

// MustMarshal is a helper function that marshals a struct into a JSON string.
// It panics if the marshal operation fails.
func MustMarshal(val any) []byte {
	encoded, err := Marshal(val)
	if err != nil {
		panic(err)
	}
	return encoded
}
