package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"github.com/neutralinsomniac/obsdpkgup/openbsd"
	version2 "github.com/neutralinsomniac/obsdpkgup/openbsd/version"
	"io/ioutil"
	"net/http"
	os "os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"suah.dev/protect"
)

// PkgVer represents an individual entry in our package index
type PkgVer struct {
	name      string
	fullName  string
	version   version2.Version
	flavor    string
	signature string
	pkgpath   string
	isBranch  bool
}

func (p PkgVer) Equals(o PkgVer) bool {
	if p.signature != o.signature {
		return false
	}

	return true
}

func (p PkgVer) String() string {
	return p.fullName
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

// returns: pkgVer struct, error
func NewPkgVerFromString(pkgStr string) (PkgVer, error) {
	pkgFileSlice := strings.Split(pkgStr, "-")
	// pkgFileSlice: "[x, y, 1.2.3p4, flavor1, flavor2]"
	// walk backwards until we find the version
	for i := len(pkgFileSlice) - 1; i >= 0; i-- {
		// found version!
		if numRE.MatchString(pkgFileSlice[i]) {
			version := version2.NewVersionFromString(pkgFileSlice[i])
			flavor := ""
			if len(pkgFileSlice[i:]) > 1 {
				flavor = strings.Join(pkgFileSlice[i+1:], "-")
			}
			return PkgVer{
				fullName: pkgStr,
				version:  version,
				flavor:   flavor,
				name:     strings.Join(pkgFileSlice[:i], "-"),
			}, nil
		}
	}
	return PkgVer{}, fmt.Errorf("couldn't find version in pkg: %q\n", pkgStr)
}

func parseLocalPkgInfoToPkgList() PkgList {
	pkgList := make(PkgList)

	pkgDbPath := "/var/db/pkg/"
	files, err := ioutil.ReadDir(pkgDbPath)
	checkAndExit(err)

	pkgpathRe := regexp.MustCompilePOSIX(`^@comment pkgpath=([^ ,]+).*$`)
	isBranchRe := regexp.MustCompilePOSIX(`^@option is-branch$`)

	for _, file := range files {
		pkgdir := file.Name()
		pkgVer, err := NewPkgVerFromString(pkgdir)
		checkAndExit(err)

		f, err := os.Open(fmt.Sprintf("%s%s/+CONTENTS", pkgDbPath, pkgdir))
		checkAndExit(err)

		contents, err := ioutil.ReadAll(f)
		checkAndExit(err)

		f.Close()

		pkgVer.signature = openbsd.GenerateSignatureFromContents(contents)
		pkgpath := pkgpathRe.FindSubmatch(contents)[1]
		pkgVer.pkgpath = string(pkgpath)
		if isBranchRe.Match(contents) {
			pkgVer.isBranch = true
		}

		pkgList[pkgVer.name] = append(pkgList[pkgVer.name], pkgVer)
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
			signature := tmp[1]
			pkgpath := tmp[2]
			pkgVer, err := NewPkgVerFromString(pkgFile[:len(pkgFile)-4])
			checkAndExit(err)
			pkgVer.signature = signature
			pkgVer.pkgpath = pkgpath
			pkgList[pkgVer.name] = append(pkgList[pkgVer.name], pkgVer)
		}
	}

	return pkgList
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
	flag.BoolVar(&forceSnapshot, "s", false, "Force checking snapshot directory for upgrades")
	flag.BoolVar(&verbose, "v", false, "Show verbose logging information")
	flag.BoolVar(&debug, "d", false, "Show debug logging information")

	flag.Parse()

	var err error

	updateList := make(map[string]bool) // this is used as a set

	mirror := getMirror()

	var allPkgs PkgList
	var sysInfo SysInfo

	sysInfo = getSystemInfo()

	pkgUpBaseUrl := os.Getenv("PKGUP_URL")
	var resp *http.Response
	var pkgUpIndexUrl string
	if pkgUpBaseUrl != "" {
		pkgUpIndexUrl = replaceMirrorVars(fmt.Sprintf("%s/%%c/%%a/index.pkgup.gz", pkgUpBaseUrl), sysInfo)
	} else {
		pkgUpIndexUrl = fmt.Sprintf("%s/index.pkgup.gz", mirror)
	}

	// grab pkgup index
	var pkgUpQuirksDateString string
	resp, err = http.Get(pkgUpIndexUrl)
	checkAndExit(err)

	switch resp.StatusCode {
	case 200:
		// grab body
		r, err := gzip.NewReader(resp.Body)
		checkAndExit(err)
		bodyBytes, err := ioutil.ReadAll(r)
		checkAndExit(err)
		resp.Body.Close()

		// get quirks timestamp from pkgUp
		quirksEndIndex := bytes.IndexByte(bodyBytes, '\n')
		pkgUpQuirksDateString = string(bodyBytes[:quirksEndIndex])
		// now parse the actual package list
		allPkgs = parseObsdPkgUpList(string(bodyBytes[quirksEndIndex:]))
	case 404:
		fmt.Fprintf(os.Stderr, "unable to locate pkgup index at '%s'.\n", pkgUpIndexUrl)
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "unexpected HTTP response: %d\n", resp.StatusCode)
		os.Exit(1)
	}

	// grab mirror quirks
	indexString, err := openbsd.GetIndexTxt(mirror)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
	mirrorQuirksSignifyBlock, err := openbsd.GetQuirksSignifyBlockFromIndex(mirror, indexString)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	// and parse the quirks date
	mirrorQuirksDateString, err := openbsd.GetSignifyTimestampFromSignifyBlock(mirrorQuirksSignifyBlock)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	pkgUpQuirksDate, err := time.Parse(openbsd.SignifyTimeFormat, pkgUpQuirksDateString)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing pkgUp quirks date %s: %s\n", pkgUpQuirksDateString, err)
		os.Exit(1)
	}

	mirrorQuirksDate, err := time.Parse(openbsd.SignifyTimeFormat, mirrorQuirksDateString)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing mirror quirks date %s: %s\n", mirrorQuirksDateString, err)
		os.Exit(1)
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
				versionComparisonResult = bestVersionMatch.version.Compare(remoteVersion.version)
				if versionComparisonResult == -1 {
					bestVersionMatch = remoteVersion
				} else if versionComparisonResult == 0 && installedVersion.signature != remoteVersion.signature {
					// check for same-version/different signature
					bestVersionMatch = remoteVersion
				}
			}

			// did we find an upgrade?
			if !bestVersionMatch.Equals(installedVersion) {
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

	if !pkgUpQuirksDate.Equal(mirrorQuirksDate) {
		if pkgUpQuirksDate.After(mirrorQuirksDate) {
			fmt.Fprintf(os.Stderr, "\nWARNING: pkgup index appears to be newer than packages on configured mirror.\n")
		} else {
			fmt.Fprintf(os.Stderr, "\nWARNING: pkgup index appears to be older than packages on configured mirror\n")
		}
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
