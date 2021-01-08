package version

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Dewey struct {
	Deweys      []string
	Suffix      string
	SuffixValue string // this is a string because technically a suffix value doesn't need to be defined (eg 1.0rc)
}

var suffixRE = regexp.MustCompile(`^(\d+)(rc|alpha|beta|pre|pl)(\d*)$`)

func NewDeweyFromString(deweyStr string) Dewey {
	dewey := Dewey{}

	dewey.Deweys = strings.Split(deweyStr, ".")

	suffixMatch := suffixRE.FindStringSubmatch(dewey.Deweys[len(dewey.Deweys)-1])
	if len(suffixMatch) != 0 {
		if suffixMatch[2] != "" {
			dewey.Deweys[len(dewey.Deweys)-1] = suffixMatch[1]
			dewey.Suffix = suffixMatch[2]
			dewey.SuffixValue = suffixMatch[3]
		}
	}

	return dewey
}

func (d Dewey) String() string {
	deweyStr := strings.Join(d.Deweys, ".")
	if len(d.Suffix) != 0 {
		deweyStr = fmt.Sprintf("%s%s%s", deweyStr, d.Suffix, d.SuffixValue)
	}

	return deweyStr
}

func (a Dewey) suffixCompare(b Dewey) int {
	if a.Suffix == b.Suffix {
		aNum, _ := strconv.Atoi(a.SuffixValue)
		bNum, _ := strconv.Atoi(b.SuffixValue)
		if aNum < bNum {
			return -1
		} else if aNum > bNum {
			return 1
		} else {
			return 0
		}
	}
	if a.Suffix == "pl" {
		return 1
	}
	if b.Suffix == "pl" {
		return -1
	}

	if a.Suffix > b.Suffix {
		return -b.suffixCompare(a)
	}

	if a.Suffix == "" {
		return 1
	}
	if a.Suffix == "alpha" {
		return -1
	}
	if a.Suffix == "beta" {
		return -1
	}

	return 0
}

func (a Dewey) Compare(b Dewey) int {
	// numerical comparison
	for i := 0; ; i++ {
		if i >= len(a.Deweys) {
			if i >= len(b.Deweys) {
				break
			} else {
				return -1
			}
		}
		if i >= len(b.Deweys) {
			return 1
		}
		r := deweyCompare(a.Deweys[i], b.Deweys[i])
		if r != 0 {
			return r
		}
	}

	return a.suffixCompare(b)
}

var versionRe = regexp.MustCompile(`^(\d+)([a-z]?)\.(\d+)([a-z]?)$`)

func deweyCompare(a, b string) int {
	aNum, err1 := strconv.Atoi(a)
	bNum, err2 := strconv.Atoi(b)

	// pure numerical comparison
	if err1 == nil && err2 == nil {
		if aNum < bNum {
			return -1
		} else if aNum > bNum {
			return 1
		} else {
			return 0
		}
	}

	cmpString := fmt.Sprintf("%s.%s", a, b)
	matches := versionRe.FindStringSubmatch(cmpString)
	if len(matches) == 5 {
		anStr, al, bnStr, bl := matches[1], matches[2], matches[3], matches[4]
		an, _ := strconv.Atoi(anStr)
		bn, _ := strconv.Atoi(bnStr)

		if an != bn {
			if an > bn {
				return 1
			} else if an < bn {
				return -1
			}
		} else {
			return strings.Compare(al, bl)
		}
	}

	return strings.Compare(a, b)
}
