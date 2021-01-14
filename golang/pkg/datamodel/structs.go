package datamodel

import (
	"database/sql"
	"time"

	"github.com/lib/pq"
)

// ParetoEntry contains the state and its corresponding total duration
type ParetoEntry struct {
	State    int
	Duration float64
}

// OrderEntry contains the begin and end time of a order
type OrderEntry struct {
	TimestampBegin time.Time
	TimestampEnd   time.Time
	OrderType      string
}

// CustomerConfiguration contains all configurations for that specific customer (incl. machine specific configurations)
type CustomerConfiguration struct {
	MicrostopDurationInSeconds                   float64
	IgnoreMicrostopUnderThisDurationInSeconds    float64
	MinimumRunningTimeInSeconds                  float64
	ThresholdForNoShiftsConsideredBreakInSeconds float64
	AvailabilityLossStates                       []int32
	PerformanceLossStates                        []int32
	LowSpeedThresholdInPcsPerHour                float64
	AutomaticallyIdentifyChangeovers             bool
	LanguageCode                                 int
}

// DataResponseAny is the format of the returned JSON.
type DataResponseAny struct {
	ColumnNames []string        `json:"columnNames"`
	Datapoints  [][]interface{} `json:"datapoints"`
}

// StateEntry contains the state and its corresponding timestamp
type StateEntry struct {
	State     int
	Timestamp time.Time
}

// CountEntry contains the count and its corresponding timestamp
type CountEntry struct {
	Count     float64
	Timestamp time.Time
}

// ShiftEntry contains the begin and end time of a shift
type ShiftEntry struct {
	TimestampBegin time.Time
	TimestampEnd   time.Time
	ShiftType      int //shiftType =0 is noShift
}

// UpcomingTimeBasedMaintenanceActivities contains information about upcoming time based maintenance activities
type UpcomingTimeBasedMaintenanceActivities struct {
	ComponentName    string
	IntervallInHours int
	ActivityType     int

	LatestActivity pq.NullTime
	NextActivity   pq.NullTime
	DurationInDays sql.NullFloat64
}

// OrdersRaw contains information about orders including their products
type OrdersRaw struct {
	OrderName   string
	TargetUnits int

	BeginTimestamp time.Time
	EndTimestamp   time.Time

	ProductName          string
	TimePerUnitInSeconds float64
}
