package openbsd

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var nameRe = regexp.MustCompilePOSIX(`^@name (.*)$`)
var dependRe = regexp.MustCompilePOSIX(`^@depend (.*)$`)
var versionRe = regexp.MustCompilePOSIX(`^@version (.*)$`)
var wantLibRe = regexp.MustCompilePOSIX(`^@wantlib (.*)$`)

var numRe = regexp.MustCompile(`^\d+.*$`)

func GenerateSignatureFromContents(contents []byte) string {
	var signatureParts []string
	// name
	matches := nameRe.FindAllSubmatch(contents, -1)
	for _, match := range matches {
		signatureParts = append(signatureParts, string(match[1]))
	}

	// version
	versionMatch := versionRe.FindSubmatch(contents)
	if len(versionMatch) > 0 {
		signatureParts = append(signatureParts, string(versionMatch[1]))
	} else {
		signatureParts = append(signatureParts, "0")
	}

	// depends
	var depends []string
	matches = dependRe.FindAllSubmatch(contents, -1)
	dependSet := make(map[string]bool)
	for _, match := range matches {
		dep := strings.Split(string(match[1]), ":")[2]
		dep = fmt.Sprintf("@%s", dep)
		dependSet[dep] = true
	}
	for dep, _ := range dependSet {
		// get the flavor part
		parts := strings.Split(dep, "-")
		flavors := make([]string, 0, len(parts))
		var i int
		for i = len(parts) - 1; !numRe.MatchString(parts[i]); i-- {
			flavors = append(flavors, parts[i])
		}
		// flavors are sorted in the signature even if they aren't in +CONTENTS
		sort.Strings(flavors)
		allParts := append(parts[:i+1], flavors...)
		dep = strings.Join(allParts, "-")
		depends = append(depends, dep)
	}
	sort.Strings(depends)

	signatureParts = append(signatureParts, depends...)

	// wantlib
	var wantlibs []string
	matches = wantLibRe.FindAllSubmatch(contents, -1)
	for _, match := range matches {
		wantlibs = append(wantlibs, string(match[1]))
	}
	sort.Strings(wantlibs)
	signatureParts = append(signatureParts, wantlibs...)

	return strings.Join(signatureParts, ",")
}
