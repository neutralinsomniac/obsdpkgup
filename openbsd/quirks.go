package openbsd

import (
	"fmt"
	"github.com/neutralinsomniac/obsdpkgup/gzip"
	"net/http"
	"strings"
)

func GetQuirksSignifyBlockFromIndex(baseUrl, index string) (string, error) {
	lines := strings.Split(index, "\n")
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		s := strings.Fields(line)
		pkgName := s[9]
		if strings.HasPrefix(pkgName, "quirks-") {
			url := fmt.Sprintf("%s%s", baseUrl, pkgName)
			resp, err := http.Get(url)
			if err != nil {
				return "", fmt.Errorf("error fetching quirks (%s): %s", url, err.Error())
			}
			defer resp.Body.Close()

			switch resp.StatusCode {
			case 200:
				gz, err := gzip.NewReader(resp.Body)
				if err != nil {
					return "", fmt.Errorf("error decompressing quirks %s: %s\n", url, err.Error())
				}

				return gz.Comment, nil
			case 404:
				return "", fmt.Errorf("404 while downloading quirks: \"%s\"\n", url)
			default:
				return "", fmt.Errorf("unexpected HTTP response (%d) while downloading quirks: %s\n", resp.StatusCode, url)
			}
		}
	}

	return "", fmt.Errorf("couldn't find quirks package in index")
}
