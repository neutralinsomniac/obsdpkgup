# obsdpkgup

```
x1$ time obsdpkgup
up to date
    0m01.69s real     0m00.51s user     0m00.25s system
```

## Pre-requisites

- Go: `pkg_add go`

## Installation

`GO111MODULE=on go get -u github.com/neutralinsomniac/obsdpkgup`

## Usage

### Check for upgrades:
`obsdpkgup`

### Run and apply found package upgrades:
`obsdpkgup |doas sh`

### Check for upgrades using signatures:

(see [The Same-Version Rebuild Problem](https://github.com/neutralinsomniac/obsdpkgup#the-same-version-rebuild-problem)):

`PKGUP_URL=https://pintobyte.com/pkgup/ obsdpkgup`

### Cron mode (don't output anything when packages are up-to-date):
`obsdpkgup -c`

## Rationale

OpenBSD's package tools are great. They've been battle-tested and designed to
correctly handle package installation/upgrades even in the face of uncertain
mirror conditions. They suffer from one problem however: with no central
package index, in order to calculate an upgrade, the signature of *every*
installed package must be checked against its signature on the configured
mirror. This process can take anywhere from 1 to over 30 minutes to complete
and uses a non-trivial amount of bandwidth, even if no updates are actually
available. A simple solution to this would be to run package upgrades overnight
through cron or some other mechanism, but not all OpenBSD machines are
always-on computers, laptops being the prime example of this.

**obsdpkgup** attempts to solve the slow `pkg_add -u` dilemma while maintaining
the consistency safeguards built into the pkgtools.

## Theory of Operation

The lack of a central-index in the packaging system is an intentional decision.
When an upgrade is requested while a mirror is in a partially-synced state, any
central index may inevitably be out-of-sync with the state of the files
present. If this index is solely relied on as a source of truth, then Bad
Things can happen when an upgrade is attempted.

Because a central index may not accurately reflect the current state of a
mirror, **obsdpkgup** uses the index to seed a `pkg_add -u` call with a subset
of packages it *thinks* have upgrades (with version numbers removed, to allow
for flexibility in the mirror state). This prevents `pkg_add` from having to
check every installed package individually, and instead enables it to check a
much smaller list of potential upgrade candidates. This way, `pkg_add` will
either do what it would've done with a full `pkg_add -u` call anyway (but with
a much smaller list of packages to check), or in the case of a de-synced index,
maybe report that a subset of the package candidate(s) are up-to-date, which
will have no adverse effect on the system.

## The Same-Version Rebuild Problem

Currently, the only index-like file available for **obsdpkgup** to check is a
mirror's `index.txt` file. This file contains the external-facing version
numbers of the packages available on the mirror, but does not reveal the
internal openbsd-specific version that can be incremented in certain
situations. When packages are rebuilt, and their external-facing versions
aren't incremented, **obsdpkgup** can't tell that an upgrade occurred. Thus, a
new index file format was created that contains a hash of a package's
"signature" (the same thing that the pkgtools themselves check) and stores it
in a secondary index file (called index.pkgup.gz). This file can be easily
generated in an existing mirror directory with the
`generate_pkgup_from_current_dir.sh` script, or generated externally to a
mirror with the `generate_pkgup_from_mirror.sh` script.
