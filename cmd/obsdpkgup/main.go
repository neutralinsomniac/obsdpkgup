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
	"strings"
	"time"

	"github.com/mcuadros/go-version"
	"suah.dev/protect"
)

// PkgVer represents an individual entry in our package index
type PkgVer struct {
	name     string
	fullName string
	version  string
	flavor   string
	hash     string
	pkgpath  string
	isBranch bool
}

// PkgList maps a package name to a list of PkgVer's
type PkgList map[string][]PkgVer

func checkAndExit(e error) {
	if e != nil {
		fmt.Fprintf(os.Stderr, "%s\n", e)
		os.Exit(1)
	}
}

var numRE = regexp.MustCompile(`^[0-9.]+.*$`)

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

	sigRe := regexp.MustCompilePOSIX(`^@name .*$|^@depend .*$|^@version .*$|^@wantlib .*$`)
	pkgpathRe := regexp.MustCompilePOSIX(`^@comment pkgpath=([^ ,]+).*$`)
	isBranchRe := regexp.MustCompilePOSIX(`^@option is-branch$`)

	for _, file := range files {
		pkgdir := file.Name()
		pkgVer, err := convertPkgStringToPkgVer(pkgdir)
		checkAndExit(err)

		f, err := os.Open(fmt.Sprintf("%s%s/+CONTENTS", pkgDbPath, pkgdir))
		checkAndExit(err)

		contents, err := ioutil.ReadAll(f)
		checkAndExit(err)

		f.Close()

		matches := sigRe.FindAll(contents, -1)

		var dataToHash []byte
		for _, match := range matches {
			dataToHash = append(dataToHash, match...)
			dataToHash = append(dataToHash, '\n')
		}

		sha256sum := sha256.Sum256(dataToHash)
		hash := base64.StdEncoding.EncodeToString(sha256sum[:])
		pkgVer.hash = hash
		pkgpath := pkgpathRe.FindSubmatch(contents)[1]
		pkgVer.pkgpath = string(pkgpath)
		if isBranchRe.Match(contents) {
			pkgVer.isBranch = true
		}

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
			pkgpath := tmp[2]
			pkgVer, err := convertPkgStringToPkgVer(pkgFile[:len(pkgFile)-4])
			checkAndExit(err)
			pkgVer.hash = hash
			pkgVer.pkgpath = pkgpath
			pkgList[pkgVer.name] = append(pkgList[pkgVer.name], *pkgVer)
		}
	}

	return pkgList
}

func compareVersionString(v1, v2 string) int {
	return version.CompareSimple(v2, v1)
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
	trustedPkgPath := os.Getenv("TRUSTED_PKG_PATH")
	if trustedPkgPath != "" {
		return replaceMirrorVars(trustedPkgPath, sysInfo)
	}

	// PKG_PATH is tested next
	pkgPath := os.Getenv("PKG_PATH")
	if pkgPath != "" {
		return replaceMirrorVars(pkgPath, sysInfo)
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
var verbose bool
var debug bool

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
	flag.BoolVar(&verbose, "v", false, "Show verbose logging information")
	flag.BoolVar(&debug, "d", false, "Show debug logging information")

	flag.Parse()

	var err error

	updateList := make(map[string]bool) // this is used as a set

	mirror := getMirror()

	var allPkgs PkgList
	var sysInfo SysInfo

	if !disablePkgUp {
		sysInfo = getSystemInfo()

		pkgupUrl := os.Getenv("PKGUP_URL")
		var resp *http.Response
		var url string
		if pkgupUrl != "" {
			url = replaceMirrorVars(fmt.Sprintf("%s/%%c/%%a/index.pkgup.gz", pkgupUrl), sysInfo)
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

	if verbose {
		fmt.Fprintf(os.Stderr, "network took: %f seconds\n", float64(time.Now().Sub(start))/float64(time.Second))
		start = time.Now()
	}

	installedPkgs := parseLocalPkgInfoToPkgList()
	var sortedInstalledPkgs []string
	for k := range installedPkgs {
		sortedInstalledPkgs = append(sortedInstalledPkgs, k)
	}
	sort.Strings(sortedInstalledPkgs)

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

		// check all versions to find upgrades
		for _, installedVersion := range installedVersions {
			versionComparisonResult := 0
			bestVersionMatch := installedVersion
		NEXTVERSION:
			for _, remoteVersion := range allPkgs[name] {
				// verify flavor/pkgpath match first
				if remoteVersion.flavor != installedVersion.flavor || remoteVersion.pkgpath != installedVersion.pkgpath {
					continue NEXTVERSION
				}

				// check for version bump
				versionComparisonResult = compareVersionString(bestVersionMatch.version, remoteVersion.version)
				if versionComparisonResult == 1 {
					bestVersionMatch = remoteVersion
				} else if versionComparisonResult == 0 && installedVersion.hash != remoteVersion.hash {
					// check for same-version/different signature
					bestVersionMatch = remoteVersion
				}

			}

			// did we find an upgrade?
			if bestVersionMatch != installedVersion {
				var index string
				if installedVersion.isBranch {
					index = fmt.Sprintf("%s%%%s", name, installedVersion.pkgpath)
				} else {
					index = name
				}
				updateList[index] = true
				fmt.Fprintf(os.Stderr, "%s->%s", installedVersion.fullName, bestVersionMatch.version)
				if installedVersion.flavor != "" {
					fmt.Fprintf(os.Stderr, "-%s", installedVersion.flavor)
				}
				fmt.Fprintf(os.Stderr, "\n")
			}
		}
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "parse took: %f seconds\n", float64(time.Now().Sub(start))/float64(time.Second))
	}

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
