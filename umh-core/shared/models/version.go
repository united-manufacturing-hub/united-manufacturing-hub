package models

type CompanionVersion struct {
	Companion Companion `definitions:"companion"`
}

type Companion struct {
	Versions []Version `definitions:"versions"`
}

type Version struct {
	Semver                     string    `definitions:"semver"`
	Changelog                  Changelog `definitions:"changelog"`
	RequiresManualIntervention bool      `definitions:"requiresManualIntervention"`
}

type Changelog struct {
	Full  []string `definitions:"full"`
	Short string   `definitions:"short"`
}
