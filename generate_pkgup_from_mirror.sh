#!/bin/sh

IFS="
"

mirror=$(cat /etc/installurl)

if [ -z "$arch" ]
then
	arch=$(arch -s)
fi

if [ -z "$version" ]
then
	if (sysctl -n kern.version |egrep -- '-current|-beta' > /dev/null)
	then
		version="snapshots"
	else
		version=$(sysctl -n kern.version |head -n 1 |awk '{print $2}')
	fi
fi

if [ a$version = asnapshots ]
then
	url="$mirror/$version/packages/$arch"
else
	url="$mirror/$version/packages-stable/$arch"
fi

index=$(curl $url/index.txt 2>/dev/null)

rm -f index.pkgup

for line in $index
do
	pkg=$(echo "$line" |awk '{print $10}')
	if echo $pkg |grep '\.tgz$'
	then
		curl $url/$pkg |tar xzqf - +CONTENTS
		hash=$(cat +CONTENTS |egrep '^@name|^@depend|^@version|^@wantlib' |sha256 -b)
		pkgpath=$(cat +CONTENTS |grep '^@comment pkgpath' |cut -b 18- |sed -e 's/[, ].*//')
		echo "$pkg" "$hash" "$pkgpath" >> index.pkgup
	fi
done

rm -f index.pkgup.gz +CONTENTS
gzip index.pkgup
