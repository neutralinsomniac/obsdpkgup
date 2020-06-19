package main

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// PkgVer represents an individual entry in our package index
type PkgVer struct {
	fullName string
	version  string
	flavor   string
	hash     string
}

// PkgList maps a package name to a PkgVer
type PkgList map[string][]PkgVer

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func remove(slice []string, s int) []string {
	return append(slice[:s], slice[s+1:]...)
}

// returns: package shortname, pkgVer struct
func convertPkgStringToPkgVer(pkgStr string) (string, PkgVer) {
	pkgFileSlice := strings.Split(pkgStr, "-")
	// pkgFileSlice: "[x, y, 1.2.3p4, flavor1, flavor2]"
	// walk backwards until we find the version
	pkgVersion := ""
	matchRE := regexp.MustCompile(`^[0-9\.]+.*$`)
	for i := len(pkgFileSlice) - 1; i >= 0; i-- {
		// found version!
		if matchRE.MatchString(pkgFileSlice[i]) {
			pkgVersion = pkgFileSlice[i]
			flavor := ""
			if len(pkgFileSlice[i:]) > 1 {
				flavor = strings.Join(pkgFileSlice[i+1:], "-")
			}
			return strings.Join(pkgFileSlice[:i], "-"), PkgVer{fullName: pkgStr, version: pkgVersion, flavor: flavor}
		}
	}
	panic("couldn't find version in pkg: " + pkgStr)
}

func parseLocalPkgInfoToPkgList() PkgList {
	pkgList := make(PkgList)

	pkgDbPath := "/var/db/pkg/"
	cmd := exec.Command("ls", "-1", pkgDbPath)
	output, err := cmd.Output()
	check(err)

	re := regexp.MustCompile(`^@name .*|^@version .*|^@wantlib .*`)
	for _, pkgdir := range strings.Split(string(output), "\n") {
		if pkgdir == "" {
			continue
		}
		name, pkgVer := convertPkgStringToPkgVer(pkgdir)

		f, err := os.Open(fmt.Sprintf("%s%s/+CONTENTS", pkgDbPath, pkgdir))
		check(err)

		scanner := bufio.NewScanner(f)
		var data_to_hash []byte
		for scanner.Scan() {
			line := scanner.Text()
			if re.MatchString(line) {
				data_to_hash = append(data_to_hash, []byte(line)...)
				data_to_hash = append(data_to_hash, '\n')
			}
		}

		f.Close()
		sha256sum := sha256.Sum256(data_to_hash)
		hash := base64.StdEncoding.EncodeToString(sha256sum[:])
		pkgVer.hash = hash
		pkgList[name] = append(pkgList[name], pkgVer)
	}
	return pkgList
}

func parseIndexToPkgList(index string) PkgList {
	pkgList := make(PkgList)

	for _, line := range strings.Split(index, "\n") {
		if len(line) > 1 {
			tmp := strings.Fields(line)
			pkgFile := tmp[len(tmp)-1]
			if !strings.HasSuffix(pkgFile, ".tgz") {
				continue
			}
			name, pkgVer := convertPkgStringToPkgVer(pkgFile[:len(pkgFile)-4])
			pkgList[name] = append(pkgList[name], pkgVer)
		}
	}
	return pkgList
}

func parseObsdPkgUpList(pkgup string) PkgList {
	pkgList := make(PkgList)

	for _, line := range strings.Split(pkgup, "\n") {
		if len(line) > 1 {
			tmp := strings.Fields(line)
			pkgFile := tmp[0]
			if !strings.HasSuffix(pkgFile, ".tgz") {
				continue
			}
			hash := tmp[1]
			name, pkgVer := convertPkgStringToPkgVer(pkgFile[:len(pkgFile)-4])
			pkgVer.hash = hash
			pkgList[name] = append(pkgList[name], pkgVer)
		}
	}

	return pkgList
}

func min(a, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}

func compareVersionString(a, b string) (ret int) {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	loopMax := len(bs)
	if len(as) > len(bs) {
		loopMax = len(as)
	}
	for i := 0; i < loopMax; i++ {
		var x, y string
		if len(as) > i {
			x = as[i]
		}
		if len(bs) > i {
			y = bs[i]
		}
		xi, _ := strconv.Atoi(x)
		yi, _ := strconv.Atoi(y)
		if xi > yi {
			ret = -1
		} else if xi < yi {
			ret = 1
		}
		if ret != 0 {
			break
		}
	}
	return
}

var cronMode bool
var disablePkgUp bool

func main() {
	flag.BoolVar(&cronMode, "c", false, "Cron mode")
	flag.BoolVar(&disablePkgUp, "n", false, "Disable pkgup index")

	flag.Parse()

	updateList := make(map[string]bool) // this is used as a set

	installurlBytes, err := ioutil.ReadFile("/etc/installurl")
	check(err)

	installurl := strings.TrimSpace(string(installurlBytes))

	cmd := exec.Command("sysctl", "-n", "kern.version")
	output, err := cmd.Output()
	check(err)

	openBSDVersion := ""
	if strings.Contains(string(output), "-current") {
		openBSDVersion = "snapshots"
	} else {
		openBSDVersion = string(output[8:11])
	}

	cmd = exec.Command("arch", "-s")
	output, err = cmd.Output()
	check(err)

	arch := strings.TrimSpace(string(output))

	var allPkgs PkgList

	if !disablePkgUp {
		resp, err := http.Get(fmt.Sprintf("%s/%s/packages/%s/index.pkgup.gz", installurl, openBSDVersion, arch))
		check(err)
		defer resp.Body.Close()

		switch resp.StatusCode {
		case 200:
			// grab body
			fmt.Fprintf(os.Stderr, "Using pkgup index\n")
			r, err := gzip.NewReader(resp.Body)
			check(err)
			bodyBytes, err := ioutil.ReadAll(r)
			check(err)
			allPkgs = parseObsdPkgUpList(string(bodyBytes))
		case 404:
			// do nothing
		default:
			panic(fmt.Sprintf("unexpected response: %d", resp.StatusCode))
		}
	}

	// if we didn't find the "new style" package list yet, fallback to old style
	if len(allPkgs) == 0 {
		resp, err := http.Get(fmt.Sprintf("%s/%s/packages/%s/index.txt", installurl, openBSDVersion, arch))
		check(err)
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			panic(fmt.Sprintf("unexpected response: %d", resp.StatusCode))
		}

		bodyBytes, err := ioutil.ReadAll(resp.Body)
		check(err)

		allPkgs = parseIndexToPkgList(string(bodyBytes))
	}

	installedPkgs := parseLocalPkgInfoToPkgList()
	var sortedInstalledPkgs []string
	for k := range installedPkgs {
		sortedInstalledPkgs = append(sortedInstalledPkgs, k)
	}
	sort.Strings(sortedInstalledPkgs)

NEXTPACKAGE:
	for _, name := range sortedInstalledPkgs {
		// quirks is treated specially; don't ever try to manually update it
		if name == "quirks" {
			continue
		}

		// if package name doesn't exist in remote, skip it
		if _, ok := allPkgs[name]; !ok {
			continue
		}

		installedVersions := installedPkgs[name]

		// check all versions to find the "closest" match
		for _, installedVersion := range installedVersions {
			// figure out our "best" version match
			var bestVersionMatch PkgVer
			bestMatchLen := -1
		NEXTVERSION:
			for _, remoteVersion := range allPkgs[name] {
				// verify flavor match first
				if remoteVersion.flavor != installedVersion.flavor {
					continue NEXTVERSION
				}
				// now find the version that matches our current version the closest
				for i := 0; i < min(len(remoteVersion.version), len(installedVersion.version)); i++ {
					if remoteVersion.version[i] != installedVersion.version[i] {
						continue NEXTVERSION
					}
					if i > bestMatchLen {
						bestMatchLen = i
						bestVersionMatch = remoteVersion
					}
				}
			}

			// we didn't find a match :<
			if bestVersionMatch.fullName == "" {
				fmt.Fprintf(os.Stderr, "WARN: couldn't find a version candidate for %s (unknown flavor?)\n", installedVersion.fullName)
				continue NEXTPACKAGE
			}

			versionComparisonResult := compareVersionString(installedVersion.version, bestVersionMatch.version)

			switch {
			case versionComparisonResult > 0:
				// version was changed; straight upgrade
				updateList[name] = true
				fmt.Fprintf(os.Stderr, "%s->%s", installedVersion.fullName, bestVersionMatch.version)
				if installedVersion.flavor != "" {
					fmt.Fprintf(os.Stderr, "-%s", installedVersion.flavor)
				}
				fmt.Fprintf(os.Stderr, "\n")
			case versionComparisonResult == 0:
				// version is the same; check sha
				if bestVersionMatch.hash != "" && installedVersion.hash != bestVersionMatch.hash {
					updateList[name] = true
					fmt.Fprintf(os.Stderr, "%s->%s", installedVersion.fullName, bestVersionMatch.version)
					if installedVersion.flavor != "" {
						fmt.Fprintf(os.Stderr, "-%s", installedVersion.flavor)
					}
					fmt.Fprintf(os.Stderr, "\n")
				}
			}
		}
	}

	if len(updateList) == 0 {
		if !cronMode {
			fmt.Fprintf(os.Stderr, "up to date\n")
		}
	} else {
		fmt.Fprintf(os.Stderr, "\nto upgrade:\n")
		fmt.Printf("pkg_add -u")
		var sortedUpdates []string
		for k := range updateList {
			sortedUpdates = append(sortedUpdates, k)
		}
		sort.Strings(sortedUpdates)
		for _, p := range sortedUpdates {
			fmt.Printf(" %s", p)
		}
		fmt.Printf("\n")
	}
}
