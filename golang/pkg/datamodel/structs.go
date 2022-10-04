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
	AvailabilityLossStates                       []int32
	PerformanceLossStates                        []int32
	MicrostopDurationInSeconds                   float64
	IgnoreMicrostopUnderThisDurationInSeconds    float64
	MinimumRunningTimeInSeconds                  float64
	ThresholdForNoShiftsConsideredBreakInSeconds float64
	LowSpeedThresholdInPcsPerHour                float64
	LanguageCode                                 LanguageCode
	AutomaticallyIdentifyChangeovers             bool
}

// DataResponseAny is the format of the returned JSON.
type DataResponseAny struct {
	ColumnNames []string        `json:"columnNames"`
	Datapoints  [][]interface{} `json:"datapoints"`
}

// StateEntry contains the state and its corresponding timestamp
type StateEntry struct {
	Timestamp time.Time
	State     int
}

// CountEntry contains the count and its corresponding timestamp
type CountEntry struct {
	Timestamp time.Time
	Count     float64
	Scrap     float64
}

// ShiftEntry contains the begin and end time of a shift
type ShiftEntry struct {
	TimestampBegin time.Time
	TimestampEnd   time.Time
	ShiftType      int // shiftType =0 is noShift
}

// UpcomingTimeBasedMaintenanceActivities contains information about upcoming time based maintenance activities
type UpcomingTimeBasedMaintenanceActivities struct {
	LatestActivity   pq.NullTime
	NextActivity     pq.NullTime
	ComponentName    string
	DurationInDays   sql.NullFloat64
	IntervallInHours int
	ActivityType     int
}

// OrdersRaw contains information about orders including their products
type OrdersRaw struct {
	BeginTimestamp       time.Time
	EndTimestamp         time.Time
	OrderName            string
	ProductName          string
	TargetUnits          int
	TimePerUnitInSeconds float64
}

// OrdersUnstartedRaw contains information about orders including their products
type OrdersUnstartedRaw struct {
	OrderName            string
	ProductName          string
	TargetUnits          int
	TimePerUnitInSeconds float64
}
