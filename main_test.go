package main

import "testing"

func TestVersionComparison(t *testing.T) {
	var res, expected int

	expected = 1
	res = compareVersionString("80", "81")
	if res != expected {
		t.Errorf("expected: %d, got: %d", expected, res)
	}

	expected = 2
	res = compareVersionString("80.1", "80.2")
	if res != expected {
		t.Errorf("expected: %d, got: %d", expected, res)
	}

	expected = 3
	res = compareVersionString("80.0.0", "80.0.1")
	if res != expected {
		t.Errorf("expected: %d, got: %d", expected, res)
	}

	expected = 1
	res = compareVersionString("80.1", "81.2")
	if res != expected {
		t.Errorf("expected: %d, got: %d", expected, res)
	}

	expected = 2
	res = compareVersionString("80.1a", "80.1b")
	if res != expected {
		t.Errorf("expected: %d, got: %d", expected, res)
	}

	expected = 2
	res = compareVersionString("80.1a", "80.1aa")
	if res != expected {
		t.Errorf("expected: %d, got: %d", expected, res)
	}

	expected = -2
	res = compareVersionString("80.1a", "80.0aa")
	if res != expected {
		t.Errorf("expected: %d, got: %d", expected, res)
	}

	expected = 3
	res = compareVersionString("80.1", "80.1.1")
	if res != expected {
		t.Errorf("expected: %d, got: %d", expected, res)
	}

	expected = -3
	res = compareVersionString("80.1.1", "80.1")
	if res != expected {
		t.Errorf("expected: %d, got: %d", expected, res)
	}

	expected = 2
	res = compareVersionString("80.1aa", "80.1b")
	if res != expected {
		t.Errorf("expected: %d, got: %d", expected, res)
	}

	expected = 2
	res = compareVersionString("80.1p1", "80.1p2")
	if res != expected {
		t.Errorf("expected: %d, got: %d", expected, res)
	}

	expected = 2
	res = compareVersionString("80.1", "80.1p1")
	if res != expected {
		t.Errorf("expected: %d, got: %d", expected, res)
	}

	expected = -1
	res = compareVersionString("82.0", "81.0.2")
	if res != expected {
		t.Errorf("expected: %d, got: %d", expected, res)
	}
}
