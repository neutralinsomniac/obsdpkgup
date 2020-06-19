#!/bin/sh
mirror=$(cat /etc/installurl)
arch=$(uname -m)

if (sysctl kern.version |grep -- -current)
then
	version="snapshots"
else
	version=$(sysctl -n kern.version |head -n 1 |cut -b 9-11)
fi

url=$mirror/$version/packages/$arch

index=$(curl $url/index.txt)

rm -f index.pkgup

IFS="
"

for line in $index
do
	pkg=$(echo $line |cut -b 53-)
	if echo $pkg |grep '\.tgz$'
	then
		curl $url/$pkg |tar xzqf - +CONTENTS
		hash=$(cat +CONTENTS |egrep '^@name|^@version|^@wantlib' |sha256 -b)
		echo $pkg $hash >> index.pkgup
		rm -f +CONTENTS
	fi
done

rm -f index.pkgup.gz
gzip index.pkgup
