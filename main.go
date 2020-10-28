package main

import (
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
	"time"

	"suah.dev/protect"
)

// PkgVer represents an individual entry in our package index
type PkgVer struct {
	name     string
	fullName string
	version  string
	flavor   string
	hash     string
}

// PkgList maps a package name to a list of PkgVer's
type PkgList map[string][]PkgVer

func checkAndExit(e error) {
	if e != nil {
		fmt.Fprintf(os.Stderr, "%s\n", e)
		os.Exit(1)
	}
}

var numRE = regexp.MustCompile(`^[0-9\.]+.*$`)

// returns: package shortname, pkgVer struct
func convertPkgStringToPkgVer(pkgStr string) (*PkgVer, error) {
	pkgFileSlice := strings.Split(pkgStr, "-")
	// pkgFileSlice: "[x, y, 1.2.3p4, flavor1, flavor2]"
	// walk backwards until we find the version
	pkgVersion := ""
	for i := len(pkgFileSlice) - 1; i >= 0; i-- {
		// found version!
		if numRE.MatchString(pkgFileSlice[i]) {
			pkgVersion = pkgFileSlice[i]
			flavor := ""
			if len(pkgFileSlice[i:]) > 1 {
				flavor = strings.Join(pkgFileSlice[i+1:], "-")
			}
			return &PkgVer{
				fullName: pkgStr,
				version:  pkgVersion,
				flavor:   flavor,
				name:     strings.Join(pkgFileSlice[:i], "-"),
			}, nil
		}
	}
	return nil, fmt.Errorf("couldn't find version in pkg: %q\n", pkgStr)
}

func parseLocalPkgInfoToPkgList() PkgList {
	pkgList := make(PkgList)

	pkgDbPath := "/var/db/pkg/"
	files, err := ioutil.ReadDir(pkgDbPath)
	checkAndExit(err)

	re := regexp.MustCompilePOSIX(`^@name .*$|^@depend .*$|^@version .*$|^@wantlib .*$`)

	for _, file := range files {
		pkgdir := file.Name()
		pkgVer, err := convertPkgStringToPkgVer(pkgdir)
		checkAndExit(err)

		f, err := os.Open(fmt.Sprintf("%s%s/+CONTENTS", pkgDbPath, pkgdir))
		checkAndExit(err)

		contents, err := ioutil.ReadAll(f)
		checkAndExit(err)

		matches := re.FindAll(contents, -1)

		var data_to_hash []byte
		for _, match := range matches {
			data_to_hash = append(data_to_hash, match...)
			data_to_hash = append(data_to_hash, '\n')
		}

		f.Close()
		sha256sum := sha256.Sum256(data_to_hash)
		hash := base64.StdEncoding.EncodeToString(sha256sum[:])
		pkgVer.hash = hash
		pkgList[pkgVer.name] = append(pkgList[pkgVer.name], *pkgVer)
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
			pkgVer, err := convertPkgStringToPkgVer(pkgFile[:len(pkgFile)-4])
			checkAndExit(err)
			pkgList[pkgVer.name] = append(pkgList[pkgVer.name], *pkgVer)
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
			pkgVer, err := convertPkgStringToPkgVer(pkgFile[:len(pkgFile)-4])
			checkAndExit(err)
			pkgVer.hash = hash
			pkgList[pkgVer.name] = append(pkgList[pkgVer.name], *pkgVer)
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
	// early escape
	if v1 == v2 {
		return 0
	}

	v1s := strings.Split(v1, ".")
	v2s := strings.Split(v2, ".")
	min := min(len(v1s), len(v2s))

	for i := 0; i < min; i++ {
		// first, snag and compare the int portions
		v1str := numberRe.FindString(v1s[i])
		v2str := numberRe.FindString(v2s[i])
		if v1str != "" && v2str != "" {
			v1num, _ := strconv.Atoi(v1str)
			v2num, _ := strconv.Atoi(v2str)
			if v1num > v2num {
				return -(i + 1)
			} else if v1num < v2num {
				return i + 1
			}
		}

		// now try alphanumeric
		if v1s[i] > v2s[i] {
			return -(i + 1)
		} else if v1s[i] < v2s[i] {
			return i + 1
		}

		// now try length
		if len(v1s[i]) > len(v2s[i]) {
			return -(i + 1)
		} else if len(v1s[i]) < len(v2s[i]) {
			return i + 1
		}
	}

	// if we got here, then we have a complete prefix match up to the common length of the split arrays.
	// return the difference in lengths to make sure that one array isn't longer than the other (to account for e.g. 81.0->81.0.2)
	if len(v2s) > len(v1s) {
		return len(v2s)
	} else if len(v1s) > len(v2s) {
		return -len(v1s)
	} else {
		return 0
	}
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
	checkAndExit(err)

	if strings.Contains(string(output), "-current") || strings.Contains(string(output), "-beta") || forceSnapshot {
		sysInfo.snapshot = true
	}

	sysInfo.version = string(output[8:11])

	cmd = exec.Command("arch", "-s")
	output, err = cmd.Output()
	checkAndExit(err)

	sysInfo.arch = strings.TrimSpace(string(output))

	return sysInfo
}

func replaceMirrorVars(mirror string, sysInfo SysInfo) string {
	if sysInfo.snapshot {
		mirror = strings.ReplaceAll(mirror, "%m", "/pub/OpenBSD/%c/packages/%a/")
	} else {
		mirror = strings.ReplaceAll(mirror, "%m", "/pub/OpenBSD/%c/packages-stable/%a/")
	}
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
		if sysInfo.snapshot {
			return replaceMirrorVars(fmt.Sprintf("%s/%%c/packages/%%a/", installurl), sysInfo)
		} else {
			return replaceMirrorVars(fmt.Sprintf("%s/%%c/packages-stable/%%a/", installurl), sysInfo)
		}
	}

	// finally, fall back to cdn
	if sysInfo.snapshot {
		return replaceMirrorVars("https://cdn.openbsd.org/pub/OpenBSD/%%c/packages/%%a/", sysInfo)
	} else {
		return replaceMirrorVars("https://cdn.openbsd.org/pub/OpenBSD/%%c/packages-stable/%%a/", sysInfo)
	}
}

var cronMode bool
var disablePkgUp bool
var forceSnapshot bool

func main() {
	start := time.Now()
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
	flag.BoolVar(&forceSnapshot, "s", false, "Force checking snapshot directory for upgrades")

	flag.Parse()

	var err error

	updateList := make(map[string]bool) // this is used as a set

	mirror := getMirror()

	var allPkgs PkgList
	var sysInfo SysInfo

	if !disablePkgUp {
		sysInfo = getSystemInfo()

		pkgup_url := os.Getenv("PKGUP_URL")
		var resp *http.Response
		var url string
		if pkgup_url != "" {
			url = replaceMirrorVars(fmt.Sprintf("%s/%%c/%%a/index.pkgup.gz", pkgup_url), sysInfo)
		} else {
			url = fmt.Sprintf("%s/index.pkgup.gz", mirror)
		}
		resp, err = http.Get(url)
		checkAndExit(err)
		defer resp.Body.Close()

		switch resp.StatusCode {
		case 200:
			// grab body
			r, err := gzip.NewReader(resp.Body)
			checkAndExit(err)
			bodyBytes, err := ioutil.ReadAll(r)
			checkAndExit(err)
			allPkgs = parseObsdPkgUpList(string(bodyBytes))
		case 404:
			fmt.Fprintf(os.Stderr, "Unable to locate pkgup index at '%s'.\nTry '%s -n' to disable pkgup index.\n", url, os.Args[0])
			os.Exit(1)
		default:
			fmt.Fprintf(os.Stderr, "unexpected HTTP response: %d\n", resp.StatusCode)
			os.Exit(1)
		}
	}

	// if we didn't find the "new style" package list yet, fallback to old style
	if len(allPkgs) == 0 {
		resp, err := http.Get(fmt.Sprintf("%s/index.txt", mirror))
		checkAndExit(err)
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			fmt.Fprintf(os.Stderr, "unexpected HTTP response: %d\n", resp.StatusCode)
			os.Exit(1)
		}

		bodyBytes, err := ioutil.ReadAll(resp.Body)
		checkAndExit(err)

		allPkgs = parseIndexToPkgList(string(bodyBytes))
	}

	fmt.Fprintf(os.Stderr, "network took: %f seconds\n", float64(time.Now().Sub(start))/float64(time.Second))
	start = time.Now()

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
			versionComparisonResult := 0
			// figure out our "best" version match
			var bestVersionMatch PkgVer
			bestMatch := -1
		NEXTVERSION:
			for _, remoteVersion := range allPkgs[name] {
				// verify flavor match first
				if remoteVersion.flavor != installedVersion.flavor {
					continue NEXTVERSION
				}
				// now find the version that matches our current version the closest
				versionComparisonResult = compareVersionString(installedVersion.version, remoteVersion.version)
				if versionComparisonResult > bestMatch && versionComparisonResult > 0 {
					bestMatch = versionComparisonResult
					bestVersionMatch = remoteVersion
				}
			}

			// we didn't find a match :<
			if bestVersionMatch.fullName == "" {
				continue NEXTPACKAGE
			}

			switch {
			case bestMatch > 0:
				// version was changed; straight upgrade
				updateList[name] = true
				fmt.Fprintf(os.Stderr, "%s->%s", installedVersion.fullName, bestVersionMatch.version)
				if installedVersion.flavor != "" {
					fmt.Fprintf(os.Stderr, "-%s", installedVersion.flavor)
				}
				fmt.Fprintf(os.Stderr, "\n")
			case bestMatch == 0:
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

	fmt.Fprintf(os.Stderr, "parse took: %f seconds\n", float64(time.Now().Sub(start))/float64(time.Second))
	if len(updateList) == 0 {
		if !cronMode {
			fmt.Fprintf(os.Stderr, "up to date\n")
		}
	} else {
		fmt.Fprintf(os.Stderr, "\nto upgrade:\n")
		fmt.Printf("pkg_add -u")
		if sysInfo.snapshot == true {
			fmt.Printf(" -Dsnap")
		}
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
