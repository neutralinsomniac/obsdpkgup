package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
)

type PkgVer struct {
	fullName string
	version  string
	flavor   string
}

// short pkg name -> []PkgVer
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
	matched := false
	for i := len(pkgFileSlice) - 1; i >= 0; i-- {
		matched, _ = regexp.MatchString(`^[0-9\.]+.*$`, pkgFileSlice[i])
		// found version!
		if matched {
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

func parsePkgInfoToPkgList(pkginfo string) PkgList {
	pkgList := make(PkgList)

	for _, line := range strings.Split(pkginfo, "\n") {
		if len(line) > 1 {
			name, pkgVer := convertPkgStringToPkgVer(line)
			// pkgFile: "x-y-1.2.3p4-flavor1-flavor2"
			pkgList[name] = append(pkgList[name], pkgVer)
		}
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

func min(a, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}

func compareVersionString(v1, v2 string) int {
	v1s := strings.Split(v1, ".")
	v2s := strings.Split(v2, ".")
	min := min(len(v1s), len(v2s))
	for i := 0; i < min; i++ {
		if v1s[i] > v2s[i] {
			return -1
		} else if v1[i] < v2[i] {
			return 1
		}
	}

	if len(v1s) > len(v2s) {
		return -1
	} else if len(v1s) < len(v2s) {
		return 1
	} else {
		return 0
	}
}

func main() {
	updateList := make(map[string]bool) // this is used as a set

	installurlBytes, err := ioutil.ReadFile("/etc/installurl")
	check(err)

	installurl := string(installurlBytes)
	installurl = strings.TrimSpace(installurl)

	cmd := exec.Command("sysctl", "-n", "kern.version")
	output, err := cmd.Output()
	check(err)

	openBSDVersion := ""
	if strings.Contains(string(output), "-current") {
		openBSDVersion = "snapshots"
	} else {
		openBSDVersion = string(output[8:11])
	}

	arch := ""
	cmd = exec.Command("uname", "-m")
	output, err = cmd.Output()
	check(err)

	arch = strings.TrimSpace(string(output))

	resp, err := http.Get(fmt.Sprintf("%s/%s/packages/%s/index.txt", installurl, openBSDVersion, arch))
	check(err)
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		panic(fmt.Sprintf("unexpected response: %d", resp.StatusCode))
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	check(err)

	body := string(bodyBytes)

	allPkgs := parseIndexToPkgList(body)
	cmd = exec.Command("ls", "-1", "/var/db/pkg/")
	output, err = cmd.Output()
	check(err)
	installedPkgs := parsePkgInfoToPkgList(string(output))

	for name, installedVersions := range installedPkgs {
		// if package name doesn't exist in remote, skip it
		if _, ok := allPkgs[name]; !ok {
			/*if !strings.HasSuffix(name, "firmware") && name != "quirks" {
				fmt.Printf("WARN: %s not in remote repo\n", name)
			}*/
			continue
		}

		// check all versions to find the "closest" match
		for _, installedVersion := range installedVersions {
			// if there's only one possible package to choose from, check it against installed
			if len(allPkgs[name]) == 1 {
				if compareVersionString(installedVersion.version, allPkgs[name][0].version) > 0 {
					updateList[name] = true
					fmt.Printf("%s -> %s\n", installedVersion.fullName, allPkgs[name][0].fullName)
				}
				continue
			} else {
				// OK, gotta figure out our "best" version match
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

				if compareVersionString(installedVersion.version, bestVersionMatch.version) > 0 {
					updateList[name] = true
					fmt.Printf("%s -> %s\n", installedVersion.fullName, bestVersionMatch.version)
				}
			}
		}
	}

	if len(updateList) == 0 {
		fmt.Println("up to date")
	} else {
		fmt.Println("to upgrade:")
		fmt.Printf("# pkg_add -u")
		for p, _ := range updateList {
			fmt.Printf(" %s", p)
		}
		fmt.Println("")
	}
}
