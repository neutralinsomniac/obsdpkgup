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
	for _, match := range matches {
		dep := strings.Split(string(match[1]), ":")[2]
		dep = fmt.Sprintf("@%s", dep)
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
