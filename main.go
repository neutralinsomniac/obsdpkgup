package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
)

type pkgList map[string]string

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
			pkgFile := strings.Fields(line)[0]
			// pkgFile: "x-y-1.2.3p4-flavor1-flavor2"
			pkgFileSlice := strings.Split(pkgFile, "-")
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
			pkgName := strings.Join(pkgFileSlice[:len(pkgFileSlice)], "-")
			pkgList[pkgName] = pkgVersion
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
			pkgName := strings.Join(pkgFileSlice[:len(pkgFileSlice)], "-")
			pkgList[pkgName] = pkgVersion
		}
	}
	return pkgList
}

func main() {
	installurlBytes, err := ioutil.ReadFile("/etc/installurl")
	check(err)

	installurl := string(installurlBytes)
	installurl = strings.TrimSpace(installurl)

	resp, err := http.Get(fmt.Sprintf("%s/snapshots/packages/amd64/index.txt", installurl))
	check(err)
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		panic(fmt.Sprintf("unexpected response: %d", resp.StatusCode))
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	check(err)

	body := string(bodyBytes)

	allPkgs := parseIndexToPkgList(body)
	cmd := exec.Command("pkg_info")
	output, err := cmd.Output()
	check(err)
	installedPkgs := parsePkgInfoToPkgList(string(output))

	for name, version := range installedPkgs {
		if len(allPkgs[name]) == 0 {
			if !strings.HasSuffix(name, "firmware") {
				fmt.Printf("WARN: %s not in remote repo\n", name)
			}
			continue
		}

		if allPkgs[name] != version {
			fmt.Printf("%s-%s -> %s-%s\n", name, version, name, allPkgs[name])
		}
	}
}
