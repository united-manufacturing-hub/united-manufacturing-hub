package migrations

import (
	_ "embed"
	"testing"
)

func TestBuildLinkedLIst(t *testing.T) {
	list := buildLinkedList()
	list.Display()
}

func TestSliceFromVersion(t *testing.T) {
	list := buildLinkedList()
	lv := SliceBetweenVersions(&list, Semver{Major: 0, Minor: 10, Patch: 1}, Semver{Major: 0, Minor: 10, Patch: 3})
	lv.Display()
}

func TestList_Apply(t *testing.T) {
	list := buildLinkedList()
	lv := SliceBetweenVersions(&list, Semver{Major: 0, Minor: 10, Patch: 1}, Semver{Major: 0, Minor: 10, Patch: 3})
	err := lv.Apply(Semver{Major: 0, Minor: 10, Patch: 1}, nil)
	if err != nil {
		t.Fatalf("error applying migrations: %v", err)
	}
}
