package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
)

type pkgList map[string][]string

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func remove(slice []string, s int) []string {
	return append(slice[:s], slice[s+1:]...)
}

func parsePkgInfoToPkgList(pkginfo string) pkgList {
	pkgList := make(pkgList)

	for _, line := range strings.Split(pkginfo, "\n") {
		if len(line) > 1 {
			// pkgFile: "x-y-1.2.3p4-flavor1-flavor2"
			pkgFileSlice := strings.Split(line, "-")
			// pkgFileSlice: "[x, y, 1.2.3p4, flavor1, flavor2]"
			// walk backwards until we find the version
			pkgVersion := ""
			matched := false
			for i := len(pkgFileSlice) - 1; i >= 0; i-- {
				matched, _ = regexp.MatchString(`^[0-9\.]+.*$`, pkgFileSlice[i])
				if matched {
					pkgVersion = pkgFileSlice[i]
					pkgFileSlice = remove(pkgFileSlice, i)
					break
				}
			}
			if !matched {
				panic("couldn't find version in pkg: " + line)
			}
			pkgName := strings.Join(pkgFileSlice, "-")
			pkgList[pkgName] = append(pkgList[pkgName], pkgVersion)
		}
	}
	return pkgList
}

func parseIndexToPkgList(index string) pkgList {
	pkgList := make(pkgList)

	for _, line := range strings.Split(index, "\n") {
		if len(line) > 1 {
			tmp := strings.Fields(line)
			pkgFile := tmp[len(tmp)-1]
			if pkgFile[len(pkgFile)-4:] != ".tgz" {
				continue
			}
			// pkgFile: "x-y-1.2.3p4-flavor1-flavor2.tgz"
			pkgFileSlice := strings.Split(pkgFile, "-")
			// pkgFileSlice: "[x, y, 1.2.3p4, flavor1, flavor2.tgz]"
			lastItem := pkgFileSlice[len(pkgFileSlice)-1]
			pkgFileSlice[len(pkgFileSlice)-1] = lastItem[:len(lastItem)-4]
			// pkgFileSlice: "[x, y, 1.2.3p4, flavor1, flavor2]"
			// walk backwards until we find the version
			pkgVersion := ""
			matched := false
			for i := len(pkgFileSlice) - 1; i >= 0; i-- {
				matched, _ = regexp.MatchString(`^[0-9\.]+.*$`, pkgFileSlice[i])
				if matched {
					pkgVersion = pkgFileSlice[i]
					pkgFileSlice = remove(pkgFileSlice, i)
					break
				}
			}
			if !matched {
				panic("couldn't find version in pkg: " + pkgFile)
			}
			pkgName := strings.Join(pkgFileSlice, "-")
			pkgList[pkgName] = append(pkgList[pkgName], pkgVersion)
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
	foundUpdate := false

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
		if len(allPkgs[name]) == 0 {
			if !strings.HasSuffix(name, "firmware") && name != "quirks" {
				fmt.Printf("WARN: %s not in remote repo\n", name)
			}
			continue
		}

		// check all versions to find the "closest" match
		for _, installedVersion := range installedVersions {
			// if there's only one possible package to choose from, check it against installed
			if len(allPkgs[name]) == 1 {
				if compareVersionString(installedVersion, allPkgs[name][0]) > 0 {
					foundUpdate = true
					fmt.Printf("%s-%s -> %s-%s\n", name, installedVersion, name, allPkgs[name][0])
				}
				continue
			} else {
				// OK, gotta figure out our "best" version match
				bestMatch := ""
				bestMatchLen := -1
			NEXTVERSION:
				for _, remoteVersion := range allPkgs[name] {
					for i, _ := range installedVersion {
						if remoteVersion[i] != installedVersion[i] {
							continue NEXTVERSION
						}
						if i > bestMatchLen {
							bestMatchLen = i
							bestMatch = remoteVersion
						}
					}
				}

				if compareVersionString(installedVersion, bestMatch) > 0 {
					foundUpdate = true
					fmt.Printf("%s-%s -> %s-%s\n", name, installedVersion, name, bestMatch)
				}
			}
		}
	}

	if !foundUpdate {
		fmt.Println("up to date")
	}
}
