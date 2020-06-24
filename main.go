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
	"strings"

	"suah.dev/protect"
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
	files, err := ioutil.ReadDir(pkgDbPath)
	check(err)

	re := regexp.MustCompile(`^@name .*|^@depend .*|^@version .*|^@wantlib .*`)
	for _, file := range files {
		pkgdir := file.Name()
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

var numberRe = regexp.MustCompile(`^\d+`)

func compareVersionString(v1, v2 string) int {
	v1s := strings.Split(v1, ".")
	v2s := strings.Split(v2, ".")
	min := min(len(v1s), len(v2s))

	for i := 0; i < min; i++ {
		// first, snag and compare the int portions
		v1num := numberRe.FindString(v1s[i])
		v2num := numberRe.FindString(v2s[i])
		if v1num != "" && v2num != "" {
			if v1num > v2num {
				return -1
			} else if v1num < v2num {
				return 1
			}
		}

		// now try length
		if len(v1s[i]) > len(v2s[i]) {
			return -1
		} else if len(v1s[i]) < len(v2s[i]) {
			return 1
		}

		// now try alphanumeric
		if v1s[i] > v2s[i] {
			return -1
		} else if v1s[i] < v2s[i] {
			return 1
		}
	}

	return 0
}

type SysInfo struct {
	arch     string
	version  string
	snapshot bool
}

func getSystemInfo() SysInfo {
	var sysInfo SysInfo

	cmd := exec.Command("sysctl", "-n", "kern.version")
	output, err := cmd.Output()
	check(err)

	if strings.Contains(string(output), "-current") {
		sysInfo.snapshot = true
	}

	sysInfo.version = string(output[8:11])

	cmd = exec.Command("arch", "-s")
	output, err = cmd.Output()
	check(err)

	sysInfo.arch = strings.TrimSpace(string(output))

	return sysInfo
}

func replaceMirrorVars(mirror string, sysInfo SysInfo) string {
	mirror = strings.ReplaceAll(mirror, "%m", "/pub/OpenBSD/%c/packages/%a/")
	mirror = strings.ReplaceAll(mirror, "%a", sysInfo.arch)
	mirror = strings.ReplaceAll(mirror, "%v", sysInfo.version)
	if sysInfo.snapshot {
		mirror = strings.ReplaceAll(mirror, "%c", "snapshots")
	} else {
		mirror = strings.ReplaceAll(mirror, "%c", sysInfo.version)
	}

	return mirror
}

func getMirror() string {
	sysInfo := getSystemInfo()

	// TRUSTED_PKG_PATH env var is tested first
	trusted_pkg_path := os.Getenv("TRUSTED_PKG_PATH")
	if trusted_pkg_path != "" {
		return replaceMirrorVars(trusted_pkg_path, sysInfo)
	}

	// PKG_PATH is tested next
	pkg_path := os.Getenv("PKG_PATH")
	if pkg_path != "" {
		return replaceMirrorVars(pkg_path, sysInfo)
	}

	// next, try /etc/installurl
	installurlBytes, err := ioutil.ReadFile("/etc/installurl")
	if err == nil {
		installurl := strings.TrimSpace(string(installurlBytes))
		return replaceMirrorVars(fmt.Sprintf("%s/%%c/packages/%%a/", installurl), sysInfo)
	}

	// finally, fall back to cdn
	return replaceMirrorVars("https://cdn.openbsd.org/pub/OpenBSD/%%c/packages/%%a/", sysInfo)
}

var cronMode bool
var disablePkgUp bool

func main() {
	_ = protect.Pledge("stdio unveil rpath wpath cpath flock dns inet tty proc exec")
	_ = protect.Unveil("/etc/resolv.conf", "r")
	_ = protect.Unveil("/etc/installurl", "r")
	_ = protect.Unveil("/etc/ssl/cert.pem", "r")
	_ = protect.Unveil("/sbin/sysctl", "rx")
	_ = protect.Unveil("/usr/bin/arch", "rx")
	_ = protect.Unveil("/bin/ls", "rx")
	_ = protect.Unveil("/var/db/pkg", "r")

	flag.BoolVar(&cronMode, "c", false, "Cron mode (only output when updates are available)")
	flag.BoolVar(&disablePkgUp, "n", false, "Disable pkgup index (fallback to index.txt)")

	flag.Parse()

	var err error

	updateList := make(map[string]bool) // this is used as a set

	mirror := getMirror()

	var allPkgs PkgList

	if !disablePkgUp {
		pkgup_index := os.Getenv("PKGUP_INDEX")
		var resp *http.Response
		if pkgup_index != "" {
			resp, err = http.Get(pkgup_index)
		} else {
			resp, err = http.Get(fmt.Sprintf("%s/index.pkgup.gz", mirror))
		}
		check(err)
		defer resp.Body.Close()

		switch resp.StatusCode {
		case 200:
			// grab body
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
		resp, err := http.Get(fmt.Sprintf("%s/index.txt", mirror))
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
