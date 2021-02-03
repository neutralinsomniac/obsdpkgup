package openbsd

import (
	"fmt"
	"io/ioutil"
	"net/http"
)

func GetIndexTxt(mirror string) (string, error) {
	indexUrl := fmt.Sprintf("%sindex.txt", mirror)
	resp, err := http.Get(indexUrl)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var indexBytes []byte
	var indexString string
	switch resp.StatusCode {
	case 200:
		indexBytes, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		indexString = string(indexBytes)
		return indexString, nil
	case 404:
		return "", fmt.Errorf("404 encountered while downloading index: %s\n", indexUrl)
	default:
		return "", fmt.Errorf("unexpected HTTP response (%d) while downloading index: %s\n", resp.StatusCode, indexUrl)
	}
}
