# obsdpkgup

Install:
`GO111MODULE=on go get -u github.com/neutralinsomniac/obsdpkgup`

Run:
`obsdpkgup`

Run and apply found package upgrades:
`obsdpkgup |doas sh`

Cron mode (no output when everything is up-to-date):
`obsdpkgup -c`
