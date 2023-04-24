package services

import (
	"fmt"
	"testing"
	"time"
)

func TestTimeBucketToDuration(t *testing.T) {
	testMinuteString := "3m"
	testHourString := "3h"
	testDayString := "3d"
	testWeekString := "3w"
	testMonthString := "3M"
	testYearString := "3y"
	testNegativeValueString := "-3m"
	testFloatValueString := "3.5m"
	testWithoutUnitString := "3"
	testInvalidUnitString := "3J"

	var answerMinute time.Duration = 3 * time.Minute //3m0s
	var answerHour time.Duration = 3 * time.Hour     //3h0m0s
	var answerDay time.Duration = 72 * time.Hour     //72h0m0s
	var answerWeek time.Duration = 3 * 24 * 7 * time.Hour
	var answerMonth time.Duration = 3 * 24 * 31 * time.Hour
	var answerYear time.Duration = 3 * 24 * 365 * time.Hour

	resultDuration, err := timeBucketToDuration(testMinuteString)

	if resultDuration != answerMinute {
		t.Errorf("wrong convertion from timeBucket to duration (Minutes): %v to %v", testMinuteString, resultDuration)
	}
	if err != nil {
		t.Errorf("error detected %v", err)
	}

	resultDuration, err = timeBucketToDuration(testHourString)
	if resultDuration != answerHour {
		t.Errorf("wrong convertion from timeBucket to duration (Hours): %v to %v", testHourString, resultDuration)
	}
	if err != nil {
		t.Errorf("error detected %v", err)
	}

	resultDuration, err = timeBucketToDuration(testDayString)
	fmt.Printf("Input: %v, expected value: %v, result: %v", testDayString, answerDay, resultDuration)
	if resultDuration != answerDay {
		t.Errorf("wrong convertion from timeBucket to duration (Days): %v to %v", testDayString, resultDuration)
	}
	if err != nil {
		t.Errorf("error detected %v", err)
	}

	resultDuration, err = timeBucketToDuration(testWeekString)
	if resultDuration != answerWeek {
		t.Errorf("wrong convertion from timeBucket to duration (Week): %v to %v", testWeekString, resultDuration)
	}
	if err != nil {
		t.Errorf("error detected %v", err)
	}

	resultDuration, err = timeBucketToDuration(testMonthString)
	if resultDuration != answerMonth {
		t.Errorf("wrong convertion from timeBucket to duration (Month): %v to %v", testMonthString, resultDuration)
	}
	if err != nil {
		t.Errorf("error detected %v", err)
	}

	resultDuration, err = timeBucketToDuration(testYearString)
	if resultDuration != answerYear {
		t.Errorf("wrong convertion from timeBucket to duration (Year): %v to %v", testYearString, resultDuration)
	}
	if err != nil {
		t.Errorf("error detected %v", err)
	}

	resultDuration, err = timeBucketToDuration(testNegativeValueString)
	if err == nil {
		t.Errorf("No error detected for a negative input: %v", testNegativeValueString)
	}

	resultDuration, err = timeBucketToDuration(testFloatValueString)
	if err == nil {
		t.Errorf("No error detected for a float value input: %v", testFloatValueString)
	}

	resultDuration, err = timeBucketToDuration(testWithoutUnitString)
	if err == nil {
		t.Errorf("No error detected for inputs without unit: %v", testWithoutUnitString)
	}

	resultDuration, err = timeBucketToDuration(testInvalidUnitString)
	if err == nil {
		t.Errorf("No error detected for inputs with an invalid unit: %v", testInvalidUnitString)
	}

}
