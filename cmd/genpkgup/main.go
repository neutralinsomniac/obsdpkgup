package main

import (
	tar2 "archive/tar"
	"github.com/neutralinsomniac/obsdpkgup/gzip"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

func main() {
	resp, err := http.Get("http://cdn.openbsd.org/pub/OpenBSD/6.8/packages/amd64/0ad-data-0.0.23b.tgz")
	if err != nil {
		panic(err)
	}

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
		b, _ := ioutil.ReadAll(tar)
		fmt.Println(string(b))
	case 404:
		fmt.Fprintf(os.Stderr, "404")
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "unexpected HTTP response: %d\n", resp.StatusCode)
		os.Exit(1)
	}
}