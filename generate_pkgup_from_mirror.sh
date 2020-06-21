#!/bin/sh

IFS="
"

mirror=$(cat /etc/installurl)
arch=$(arch -s)

if (sysctl -n kern.version |grep -- -current > /dev/null)
then
	version="snapshots"
else
	version=$(sysctl -n kern.version |head -n 1 |cut -b 9-11)
fi

url="$mirror/$version/packages/$arch"

index=$(curl $url/index.txt 2>/dev/null)

if [ -e index.pkgup.gz ]
then
	local_quirks=$(gunzip -c index.pkgup.gz |grep quirks |cut -d " " -f 1)
	remote_quirks=$(echo "$index" |grep quirks |cut -b 53-)
	if [ a$local_quirks = a$remote_quirks ]
	then
		echo "no update required"
		exit
	fi
fi

rm -f index.pkgup

for line in $index
do
	pkg=$(echo "$line" |cut -b 53-)
	if echo $pkg |grep '\.tgz$'
	then
		curl $url/$pkg |tar xzqf - +CONTENTS
		hash=$(cat +CONTENTS |egrep '^@name|^@version|^@wantlib' |sha256 -b)
		echo "$pkg" "$hash" >> index.pkgup
	fi
done

rm -f index.pkgup.gz +CONTENTS
gzip index.pkgup
