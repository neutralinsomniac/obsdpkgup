#!/bin/sh

rm -f index.pkgup

for pkg in $(ls *.tgz)
do
	tar -C /tmp/ -xzf $pkg +CONTENTS
	echo "$pkg" $(cat /tmp/+CONTENTS |egrep '^@name|^@version|^@wantlib' |sha256 -b) >> index.pkgup
done

rm -f index.pkgup.gz
gzip -f index.pkgup
