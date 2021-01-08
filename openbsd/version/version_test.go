package version

import (
	debug2 "runtime/debug"
	"testing"
)

func checkErr(err error) {
	if err != nil {
		panic("static parsing failed??")
	}
}

func checkVersion(t *testing.T, leftStr string, rightStr string, expected int) {
	var left, right Version

	left = NewVersionFromString(leftStr)
	right = NewVersionFromString(rightStr)
	res := left.Compare(right)
	if res != expected {
		t.Errorf("expected: %d, got: %d", expected, res)
		t.Log(string(debug2.Stack()))
	}
}

func TestVersionComparison(t *testing.T) {
	// these are ripped straight out of packages-specs(7)
	checkVersion(t, "foo-1.01", "foo-1.1", 0)

	checkVersion(t, "foo-1.001", "foo-1.002", -1)

	checkVersion(t, "foo-1.002", "foo-1.0010", -1)

	checkVersion(t, "foo-1.0rc2", "foo-1.0pre3", 0)

	checkVersion(t, "bar-1.0alpha5", "bar-1.0beta3", -1)

	checkVersion(t, "bar-1.0beta3", "bar-1.0rc1", -1)

	checkVersion(t, "baz-1.0", "baz-1.0pl1", -1)

	// these ones I made up
	checkVersion(t, "foo-80", "foo-81", -1)

	checkVersion(t, "foo-1.14.7v3", "foo-1.14.7p0v3", -1)

	checkVersion(t, "foo-1.0p2", "foo-1.0v2", -1)

	checkVersion(t, "foo-80.1", "foo-80.2", -1)

	checkVersion(t, "foo-80.0.0", "foo-80.0.1", -1)

	checkVersion(t, "foo-80.0.0", "foo-81.0.1", -1)

	checkVersion(t, "foo-80.1a", "foo-80.1b", -1)

	checkVersion(t, "foo-80.1b", "foo-80.1a", 1)

	checkVersion(t, "foo-80.1", "foo-80.1.1", -1)

	checkVersion(t, "foo-80.1aa", "foo-80.1b", -1)

	checkVersion(t, "foo-80.1p1", "foo-80.1p2", -1)

	checkVersion(t, "foo-80.1", "foo-80.1p1", -1)

	checkVersion(t, "foo-80.1a", "foo-80.0aa", 1)

	checkVersion(t, "foo-80.1.1", "foo-80.1", 1)

	checkVersion(t, "foo-80.1a", "foo-80.1aa", -1)

	checkVersion(t, "foo-80.1.1", "foo-80.1.1", 0)
}
