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

func TestSemver_Compare(t *testing.T) {
	a := Semver{Major: 0, Minor: 10, Patch: 1}
	b := Semver{Major: 0, Minor: 10, Patch: 3}
	c := Semver{Major: 0, Minor: 10, Patch: 1}
	if a.Compare(b) != -1 {
		t.Fatalf("expected -1, got %d", a.Compare(b))
	}
	if b.Compare(a) != 1 {
		t.Fatalf("expected 1, got %d", b.Compare(a))
	}
	if a.Compare(a) != 0 {
		t.Fatalf("expected 0, got %d", a.Compare(a))
	}

	if a.Compare(c) != 0 {
		t.Fatalf("expected 0, got %d", a.Compare(c))
	}
}
