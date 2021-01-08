package version

import (
	"fmt"
	"regexp"
	"strconv"
)

type Version struct {
	Dewey Dewey
	P     int // always init to -1
	V     int // always init to -1
}

var vRE = regexp.MustCompile(`^(.*)v(\d+)$`)
var pRE = regexp.MustCompile(`^(.*)p(\d+)$`)

func NewVersionFromString(versionStr string) Version {
	var version Version
	version.P = -1
	version.V = -1

	matches := vRE.FindStringSubmatch(versionStr)
	if len(matches) > 0 {
		version.V, _ = strconv.Atoi(matches[2])
		versionStr = matches[1]
	}

	matches = pRE.FindStringSubmatch(versionStr)
	if len(matches) > 0 {
		version.P, _ = strconv.Atoi(matches[2])
		versionStr = matches[1]
	}

	version.Dewey = NewDeweyFromString(versionStr)

	return version
}

func (v Version) String() string {
	versionStr := v.Dewey.String()
	if v.P != -1 {
		versionStr = fmt.Sprintf("%sp%d", versionStr, v.P)
	}
	if v.V != -1 {
		versionStr = fmt.Sprintf("%sv%d", versionStr, v.V)
	}
	return versionStr
}

func (a Version) PnumCompare(b Version) int {
	if a.P < b.P {
		return -1
	} else if a.P > b.P {
		return 1
	} else {
		return 0
	}
}

func (a Version) Compare(b Version) int {
	// simple case: epoch number
	if a.V != b.V {
		if a.V < b.V {
			return -1
		} else if a.V > b.V {
			return 1
		} else {
			return 0
		}
	}
	// simple case: only p number differs
	if a.Dewey.Compare(b.Dewey) == 0 {
		return a.PnumCompare(b)
	}

	return a.Dewey.Compare(b.Dewey)
}
