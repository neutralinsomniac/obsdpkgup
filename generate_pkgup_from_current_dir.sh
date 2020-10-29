#!/bin/sh

rm -f index.pkgup

for pkg in $(ls *.tgz)
do
	tar xzqf "$pkg" +CONTENTS
	hash=$(cat +CONTENTS |egrep '^@name|^@depend|^@version|^@wantlib' |sha256 -b)
	pkgpath=$(cat +CONTENTS |grep '^@comment pkgpath' |cut -b 18- |sed -e 's/[, ].*//')
	echo "$pkg" "$hash" "$pkgpath" >> index.pkgup
done

rm -f index.pkgup.gz +CONTENTS
gzip -f index.pkgup
