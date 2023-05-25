package services

import (
	"testing"
	"time"
)

func TestTimeBucketToDuration(t *testing.T) {

	validInputOutputMap := map[string]time.Duration{
		"3m": 3 * time.Minute,
		"3h": 3 * time.Hour,
		"3d": 72 * time.Hour,
		"3w": 3 * 24 * 7 * time.Hour,
		"3M": 3 * 24 * 31 * time.Hour,
		"3y": 3 * 24 * 365 * time.Hour}

	invalidInputList := []string{"-3m", "3.5m", "3", "3J"}

	for key, value := range validInputOutputMap {
		resultDuration, err := timeBucketToDuration(key)
		if resultDuration != value {
			t.Errorf("wrong conversion from timeBucket to duration: %v to %v", key, value)
		}
		if err != nil {
			t.Errorf("error detected %v", err)
		}
	}

	for _, value := range invalidInputList {
		_, err := timeBucketToDuration(value)
		if err == nil {
			t.Errorf("No error detected for a negative input: %v", value)
		}
	}
}
