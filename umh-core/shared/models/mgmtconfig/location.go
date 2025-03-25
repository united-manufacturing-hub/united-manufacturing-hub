package mgmtconfig

import (
	"fmt"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/constants"
)

type Location struct {
	Enterprise string `json:"enterprise"`
	Site       string `json:"site"`
	Area       string `json:"area"`
	Line       string `json:"line"`
	WorkCell   string `json:"workCell"`
}

func (l *Location) String() string {
	str := fmt.Sprintf("%s.%s.%s.%s.%s", l.Enterprise, l.Site, l.Area, l.Line, l.WorkCell)
	// Remove trailing dots
	return strings.TrimRight(str, ".")
}

func (l *Location) IsEmpty() bool {
	return l.Enterprise == "" && l.Site == "" && l.Area == "" && l.Line == "" && l.WorkCell == ""
}

// Equals checks if two locations are the same up to a certain level.
// Level 0: Only compare the enterprise
// Level 1: Compare the enterprise and site
// Level 2: Compare the enterprise, site, and area
// Level 3: Compare the enterprise, site, area, and line
// Level 4: Compare the enterprise, site, area, line, and workCell
func (l *Location) Equals(other Location, level int) bool {
	switch level {
	case 0:
		return l.Enterprise == other.Enterprise
	case 1:
		return l.Enterprise == other.Enterprise && l.Site == other.Site
	case 2:
		return l.Enterprise == other.Enterprise && l.Site == other.Site && l.Area == other.Area
	case 3:
		return l.Enterprise == other.Enterprise && l.Site == other.Site && l.Area == other.Area && l.Line == other.Line
	case 4:
		return l.Enterprise == other.Enterprise && l.Site == other.Site && l.Area == other.Area && l.Line == other.Line && l.WorkCell == other.WorkCell
	default:
		return false
	}
}

// FromTopicString converts a topic string to a mgmtconfig.Location object.
// It filters out the umh.v1 prefix, originId, schema, and tag elements from the topic.
func FromTopicString(topic string) Location {
	if topic == "umh.v1" {
		return Location{}
	}
	elements := strings.Split(strings.TrimPrefix(topic, constants.TOPIC_PREFIX+"."), ".")
	// topic structure: umh.v1.<enterprise>.<site>.<area>.<productionLine>.<workCell>.<originId>.<_schema>.<tag>
	// We don't care about origin, schema, or tag, therefor we will filter them out
	//	This is done by finding the first element starting with an _ and removing it and all following elements
	//  Note: There might be none
	for i, element := range elements {
		if strings.HasPrefix(element, "_") {
			elements = elements[:i]
			break
		}
	}

	if len(elements) == 0 {
		return Location{}
	}
	location := Location{
		Enterprise: elements[0],
	}
	if len(elements) >= 2 {
		location.Site = elements[1]
	}
	if len(elements) >= 3 {
		location.Area = elements[2]
	}
	if len(elements) >= 4 {
		location.Line = elements[3]
	}
	if len(elements) >= 5 {
		location.WorkCell = elements[4]
	}

	return location
}
