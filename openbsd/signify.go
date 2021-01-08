package openbsd

import (
	"fmt"
	"strings"
	"time"
)

var SignifyTimeFormat = time.RFC3339

func GetSignifyTimestampFromSignifyBlock(signifyBlock string) (string, error) {
	lines := strings.Split(signifyBlock, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "date=") {
			return line[5:], nil
		}
	}

	return "", fmt.Errorf("could not find date in signify block")
}
