#!/bin/sh

rm -f index.pkgup

for pkg in $(ls *.tgz)
do
	# could use -O option from gtar here to output to stdout instead of writing to tmp file
	tar -C /tmp/ -xzf $pkg +CONTENTS
	echo $pkg $(cat /tmp/+CONTENTS |egrep '^@name|^@version|^@wantlib' |sha256 -b) >> index.pkgup
done

rm -f index.pkgup.gz
gzip index.pkgup
