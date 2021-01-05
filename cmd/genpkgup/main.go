package main

import (
	tar2 "archive/tar"
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"fmt"
	"github.com/neutralinsomniac/obsdpkgup/gzip"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"
)

func getContentsFromPkgUrl(url string) []byte {
	resp, err := http.Get(url)
	if err != nil {
		panic(fmt.Sprintf("%s: %s", url, err))
	}

	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			panic(err)
		}

		tar := tar2.NewReader(gz)

		hdr, err := tar.Next()
		if err != nil {
			panic(err)
		}
		for err == nil && hdr.Name != "+CONTENTS" {
			hdr, err = tar.Next()
			if err != nil {
				panic(err)
			}
		}
		contents, _ := ioutil.ReadAll(tar)
		return contents
	case 404:
		fmt.Fprintf(os.Stderr, "404")
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "unexpected HTTP response: %d\n", resp.StatusCode)
		os.Exit(1)
	}

	return []byte{}
}

var mirror string
var arch string
var version string

var sigRe = regexp.MustCompilePOSIX(`^@name .*$|^@depend .*$|^@version .*$|^@wantlib .*$`)
var pkgpathRe = regexp.MustCompilePOSIX(`^@comment pkgpath=([^ ,]+).*$`)

func main() {
	flag.StringVar(&mirror, "m", "https://cdn.openbsd.org/pub/OpenBSD", "Mirror URL")
	flag.StringVar(&arch, "a", "", "Architecture")
	flag.StringVar(&version, "v", "", "Version")

	flag.Parse()

	if version == "" {
		fmt.Fprintf(os.Stderr, "Error: Must specify version (-v)\n")
		os.Exit(1)
	}

	if arch == "" {
		fmt.Fprintf(os.Stderr, "Error: Must specify arch (-a)\n")
		os.Exit(1)
	}
	// retrieve the index.txt first
	var url string
	if version == "snapshots" {
		url = fmt.Sprintf("%s/%s/packages/%s", mirror, version, arch)
	} else {
		url = fmt.Sprintf("%s/%s/packages-stable/%s", mirror, version, arch)
	}

	resp, err := http.Get(fmt.Sprintf("%s/index.txt", url))
	if err != nil {
		panic(err)
	}

	var indexBytes []byte
	var indexString string
	switch resp.StatusCode {
	case 200:
		indexBytes, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}
		indexString = string(indexBytes)
	case 404:
		fmt.Fprintf(os.Stderr, "404")
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "unexpected HTTP response: %d\n", resp.StatusCode)
		os.Exit(1)
	}

	for _, line := range strings.Split(indexString, "\n") {
		if len(line) == 0 {
			continue
		}
		s := strings.Fields(line)
		pkgName := s[9]
		if !strings.HasSuffix(pkgName, ".tgz") {
			continue
		}
		contents := getContentsFromPkgUrl(fmt.Sprintf("%s/%s", url, pkgName))
		matches := sigRe.FindAll(contents, -1)

		var dataToHash []byte
		for _, match := range matches {
			dataToHash = append(dataToHash, match...)
			dataToHash = append(dataToHash, '\n')
		}

		sha256sum := sha256.Sum256(dataToHash)
		hash := base64.StdEncoding.EncodeToString(sha256sum[:])

		pkgPath := pkgpathRe.FindSubmatch(contents)[1]

		fmt.Printf("%s %s %s\n", pkgName, hash, pkgPath)
	}
}
