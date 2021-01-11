#!/usr/bin/env sh

pkgup_dir=/var/www/pkgup

for arch in amd64 aarch64
do
    for version in snapshots 6.8
    do
        echo $arch:$version
        if genpkgup -a $arch -v $version > index.pkgup
        then
            rm -f index.pkgup.gz
            gzip index.pkgup
            mv index.pkgup.gz $pkgup_dir/$version/$arch/
        fi
    done
done
