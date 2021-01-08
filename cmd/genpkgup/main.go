package main

import (
	tar2 "archive/tar"
	"flag"
	"fmt"
	"github.com/neutralinsomniac/obsdpkgup/gzip"
	"github.com/neutralinsomniac/obsdpkgup/openbsd"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"
)

func getContentsFromPkgUrl(url string) []byte {
	resp, err := http.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error downloading package %s: %s\n", url, err)
		goto Error
	}

	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error decompressing %s: %s\n", url, err)
			goto Error
		}

		tar := tar2.NewReader(gz)

		hdr, err := tar.Next()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error decompressing %s: %s\n", url, err)
			goto Error
		}
		for err == nil && hdr.Name != "+CONTENTS" {
			hdr, err = tar.Next()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error walking archive %s: %s\n", url, err)
				goto Error
			}
		}
		contents, _ := ioutil.ReadAll(tar)
		return contents
	case 404:
		fmt.Fprintf(os.Stderr, "404 while downloading package: %s\n", url)
		goto Error
	default:
		fmt.Fprintf(os.Stderr, "unexpected HTTP response (%d) while downloading package: %s\n", resp.StatusCode, url)
		goto Error
	}

Error:
	return []byte{}
}

var mirror string
var arch string
var version string
var showProgress bool

var pkgpathRe = regexp.MustCompilePOSIX(`^@comment pkgpath=([^ ,]+).*$`)

func main() {
	flag.StringVar(&mirror, "m", "https://cdn.openbsd.org/pub/OpenBSD", "Mirror URL")
	flag.StringVar(&arch, "a", "", "Architecture")
	flag.StringVar(&version, "v", "", "Version")
	flag.BoolVar(&showProgress, "p", false, "Show progress")

	flag.Parse()

	if version == "" {
		fmt.Fprintf(os.Stderr, "Error: Must specify version (-v)\n")
		os.Exit(1)
	}

	if arch == "" {
		fmt.Fprintf(os.Stderr, "Error: Must specify arch (-a)\n")
		os.Exit(1)
	}

	var url string
	if version == "snapshots" {
		url = fmt.Sprintf("%s/%s/packages/%s", mirror, version, arch)
	} else {
		url = fmt.Sprintf("%s/%s/packages-stable/%s", mirror, version, arch)
	}

	// retrieve the index.txt first
	indexString, err := openbsd.GetIndexTxt(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to retrieve index.txt at %s: %s", url, err)
		os.Exit(1)
	}

	// snag quirks for timestamp
	quirksSignifyBlock, err := openbsd.GetQuirksSignifyBlockFromIndex(url, indexString)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
		os.Exit(1)
	}
	quirksDate, err := openbsd.GetSignifyTimestampFromSignifyBlock(quirksSignifyBlock)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
		os.Exit(1)
	}
	fmt.Println(quirksDate)

	lines := strings.Split(indexString, "\n")
	numPkgsToProcess := len(lines)
	for i, line := range lines {
		if showProgress {
			fmt.Fprintf(os.Stderr, "\r%d/%d", i, numPkgsToProcess)
		}
		if len(line) == 0 {
			continue
		}
		s := strings.Fields(line)
		pkgName := s[9]
		// doesn't look like a package to me
		if !strings.HasSuffix(pkgName, ".tgz") {
			continue
		}
		contents := getContentsFromPkgUrl(fmt.Sprintf("%s/%s", url, pkgName))
		if len(contents) == 0 {
			// if we failed to get/decompress +CONTENTS, skip this package
			continue
		}

		signature := openbsd.GenerateSignatureFromContents(contents)

		pkgPath := pkgpathRe.FindSubmatch(contents)[1]

		fmt.Printf("%s %s %s\n", pkgName, signature, pkgPath)
	}
}
