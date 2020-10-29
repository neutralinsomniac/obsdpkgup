#!/bin/sh

current_release=6.8
host=myhost.com
pkgupdir=/var/www/pkgup

# amd64 release
export version=$current_release
export arch=amd64
./generate_pkgup_from_mirror.sh 2>/dev/null
scp index.pkgup.gz $host:$pkgupdir/$version/$arch/

# amd64 snapshots
export version=snapshots
./generate_pkgup_from_mirror.sh 2>/dev/null
scp index.pkgup.gz $host:$pkgupdir/$version/$arch/

# aarch64 release
export version=$current_release
export arch=aarch64
./generate_pkgup_from_mirror.sh 2>/dev/null
scp index.pkgup.gz $host:$pkgupdir/$version/$arch/

# aarch64 snapshots
export version=snapshots
./generate_pkgup_from_mirror.sh 2>/dev/null
scp index.pkgup.gz $host:$pkgupdir/$version/$arch/
