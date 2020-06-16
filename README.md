# obsdpkgup

```
x1$ time obsdpkgup
up to date
    0m01.69s real     0m00.51s user     0m00.25s system
```

Install:
`GO111MODULE=on go get -u github.com/neutralinsomniac/obsdpkgup`

Run:
`obsdpkgup`

Run and apply found package upgrades:
`obsdpkgup |doas sh`

Cron mode (no output when everything is up-to-date):
`obsdpkgup -c`
