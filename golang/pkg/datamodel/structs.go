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

// EnterpriseConfiguration contains all configurations for that specific enterprise (incl. machine specific configurations)
type EnterpriseConfiguration struct {
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

// DatabaseStatistics holds statistics for a database, including the size of the database in bytes and statistics for each table in the database
type DatabaseStatistics struct {
	DatabaseSizeInBytes int64
	TableStatistics     map[string]DatabaseTableStatistics
}

// DatabaseTableStatistics holds statistics for a table in a database, including the number of approximate rows, the last time the table was auto-analyzed and auto-vacuumed, and whether or not the table is a hypertable
type DatabaseTableStatistics struct {
	ApproximateRows  int64
	LastAutoAnalyze  sql.NullString
	LastAutoVacuum   sql.NullString
	LastAnalyze      sql.NullString
	LastVacuum       sql.NullString
	IsHyperTable     bool
	NormalStats      DatabaseNormalTableStatistics
	HyperStats       []DatabaseHyperTableStatistics
	HyperRetention   DatabaseHyperTableRetention
	HyperCompression DatabaseHyperTableCompression
}

// DatabaseNormalTableStatistics holds statistics for a normal table in a database, including the sizes of various components of the table
type DatabaseNormalTableStatistics struct {
	PgTableSize         int64
	PgTotalRelationSize int64
	PgIndexesSize       int64
	PgRelationSizeMain  int64
	PgRelationSizeFsm   int64
	PgRelationSizeVm    int64
	PgRelationSizeInit  int64
}

// DatabaseHyperTableStatistics holds statistics for a hypertable in a database, including the sizes of various components of the table and the name of the node hosting the table
type DatabaseHyperTableStatistics struct {
	TableBytes int64
	IndexBytes int64
	ToastBytes int64
	TotalBytes int64
	NodeName   sql.NullString
}

// DatabaseHyperTableRetention holds information about the retention policy for a hypertable
type DatabaseHyperTableRetention struct {
	ScheduleInterval string
	Config           string
}

// DatabaseHyperTableCompression holds information about the compression policy for a hypertable
type DatabaseHyperTableCompression struct {
	ScheduleInterval string
	Config           string
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

// ChannelResult returns the returnValue and an error code from a goroutine
type ChannelResult struct {
	Err         error
	ReturnValue interface{}
}
