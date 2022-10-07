package migrations

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	v_0_10 "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/automigrate/migrations/0/10"
	"go.uber.org/zap"
	"regexp"
	"strconv"
)

type Semver struct {
	Major         int
	Minor         int
	Patch         int
	Prerelease    string
	BuildMetadata string
}

func (s Semver) String() string {
	version := fmt.Sprintf("%d.%d.%d", s.Major, s.Minor, s.Patch)
	if s.Prerelease != "" {
		version += fmt.Sprintf("-%s", s.Prerelease)
	}
	if s.BuildMetadata != "" {
		version += fmt.Sprintf("+%s", s.BuildMetadata)
	}
	return version
}

func StringToSemver(version string) (semver Semver, parseable bool) {
	// strip leading v
	version = regexp.MustCompile("^v").ReplaceAllString(version, "")
	// https://semver.org/#is-there-a-suggested-regular-expression-regex-to-check-a-semver-string
	smRegex := regexp.MustCompile(`^(?P<major>0|[1-9]\d*)\.(?P<minor>0|[1-9]\d*)\.(?P<patch>0|[1-9]\d*)(?:-(?P<prerelease>(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+(?P<buildmetadata>[0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

	match := smRegex.FindStringSubmatch(version)
	if match == nil {
		zap.S().Errorf("Version %s is not a valid semver", version)
		return semver, false
	}
	result := make(map[string]string)
	for i, name := range smRegex.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = match[i]
		}
	}
	majorInt, err := strconv.ParseInt(result["major"], 10, 32)
	if err != nil {
		zap.S().Errorf("Error parsing major version: %s", err)
		return semver, false
	}

	minorInt, err := strconv.ParseInt(result["minor"], 10, 32)
	if err != nil {
		zap.S().Errorf("Error parsing minor version: %s", err)
		return semver, false
	}

	patchInt, err := strconv.ParseInt(result["patch"], 10, 32)
	if err != nil {
		zap.S().Errorf("Error parsing patch version: %s", err)
		return semver, false
	}

	prerelease, ok := result["prerelease"]
	if !ok {
		prerelease = ""
	}
	buildMetadata, ok := result["buildmetadata"]
	if !ok {
		buildMetadata = ""
	}

	return Semver{
		Major:         int(majorInt),
		Minor:         int(minorInt),
		Patch:         int(patchInt),
		Prerelease:    prerelease,
		BuildMetadata: buildMetadata,
	}, true
}

func Migrate(current Semver, db *sql.DB) {
	// Don't apply migrations on prerelease versions
	if current.Prerelease != "" {
		zap.S().Infof("Not applying migrations on prerelease version %s", current)
		return
	}

	// Check if any migration was applied
	// If not, apply all migrations
	// If yes, check if current version is higher than the last applied migration
	// If yes, apply all migrations that are higher than the last applied migration
	// If no, do nothing

	if !checkForAnyMigration(db) {
		applyMigrations(Semver{Major: 0, Minor: 0, Patch: 0}, current, db)
	} else {
		latest := getLatestMigration(db)
		applyMigrations(latest, current, db)
	}
}

func checkIfNewer(last Semver, current Semver) bool {
	if last.Major < current.Major {
		return false
	}
	if last.Major == current.Major && last.Minor < current.Minor {
		return false
	}
	if last.Major == current.Major && last.Minor == current.Minor && last.Patch <= current.Patch {
		return false
	}
	return true
}
func getLatestMigration(db *sql.DB) Semver {
	result, err := db.Query("SELECT tomajor, tominor, topatch FROM migrationtable ORDER BY timestamp DESC LIMIT 1")
	if err != nil {
		zap.S().Fatalf("Error getting latest migration: %s", err)
	}
	var major, minor, patch int
	err = result.Scan(&major, &minor, &patch)
	if err != nil {
		zap.S().Fatalf("Error getting latest migration: %s", err)
	}
	return Semver{Major: major, Minor: minor, Patch: patch}
}

func checkForAnyMigration(db *sql.DB) bool {
	result, err := db.Query("SELECT count(1) FROM migrationtable")
	if err != nil {
		zap.S().Fatalf("Error checking for any migration: %s", err)
	}
	var count int
	err = result.Scan(&count)
	if err != nil {
		zap.S().Fatalf("Error checking for any migration: %s", err)
	}
	return count > 0
}

func applyMigrations(last Semver, current Semver, db *sql.DB) {
	if !checkIfNewer(last, current) {
		zap.S().Infof("No migrations to apply")
		return
	}
	zap.S().Infof("Applying migrations from %s to %s", last, current)

	// Apply migrations
	fullList := buildLinkedList()
	applyList := SliceBetweenVersions(&fullList, last, current)
	err := applyList.Apply(last, db)
	if err != nil {
		zap.S().Fatalf("Error applying migrations: %s", err)
	}

}

type migrationfunc func(db *sql.DB) error

type Node struct {
	prev      *Node
	next      *Node
	migration migrationfunc
	version   Semver
}

type List struct {
	head *Node
	tail *Node
}

func (L *List) Insert(mfunc migrationfunc, version Semver) {
	list := &Node{
		next:      L.head,
		migration: mfunc,
		version:   version,
	}
	if L.head != nil {
		L.head.prev = list
	}
	L.head = list

	l := L.head
	for l.next != nil {
		l = l.next
	}
	L.tail = l
}

func (L *List) Display() {
	l := L.head
	for l != nil {
		fmt.Println(l.version)
		l = l.next
	}
}

func (L *List) Apply(oldVersion Semver, db *sql.DB) error {
	if db == nil {
		return errors.New("db cannot be nil")
	}
	if L.head == nil {
		return errors.New("list is empty")
	}
	l := L.head
	lastVersion := oldVersion
	for l != nil {
		zap.S().Infof("Applying migration %s", l.version)
		err := l.migration(db)
		if err != nil {
			return err
		}
		// Set migration as applied
		_, err = db.Exec(
			"INSERT INTO migrationtable (frommajor, fromminor, frompatch, tomajor, tominor, topatch, timestamp) VALUES (?, ?, ?, ?, ?, ?, now())",
			lastVersion.Major,
			lastVersion.Minor,
			lastVersion.Patch,
			l.version.Major,
			l.version.Minor,
			l.version.Patch)
		if err != nil {
			return err
		}
		lastVersion = l.version
		l = l.next
	}
	return nil
}

// SliceBetweenVersions returns a new list with all migrations between the two versions (excluding oldVersion and including newVersion)
func SliceBetweenVersions(L *List, oldVersion Semver, newVersion Semver) *List {
	var newList List
	l := L.head
	for l != nil {
		if checkIfNewer(l.version, oldVersion) {
			if !checkIfNewer(l.version, newVersion) {
				newList.Insert(l.migration, l.version)
			}
		}
		l = l.next
	}
	return &newList
}

// buildLinkedList builds a linked list of all migrations
// This is where you add new migrations
func buildLinkedList() List {
	var migrationList List

	// Add migrations here
	migrationList.Insert(v_0_10.V0x10x0, Semver{Major: 0, Minor: 10, Patch: 0})
	return migrationList
}
