PKGUP_DIR=/var/www/pkgup

ARCH=amd64
VERSION=snapshots
echo $ARCH:$VERSION
genpkgup -p -a $ARCH -v $VERSION > index.pkgup
gzip index.pkgup
mv index.pkgup.gz $PKGUP_DIR/$VERSION/$ARCH/

ARCH=aarch64
VERSION=snapshots
echo $ARCH:$VERSION
genpkgup -p -a $ARCH -v $VERSION > index.pkgup
gzip index.pkgup
mv index.pkgup.gz $PKGUP_DIR/$VERSION/$ARCH/

ARCH=amd64
VERSION=6.8
echo $ARCH:$VERSION
genpkgup -p -a $ARCH -v $VERSION > index.pkgup
gzip index.pkgup
mv index.pkgup.gz $PKGUP_DIR/$VERSION/$ARCH/

ARCH=aarch64
VERSION=6.8
echo $ARCH:$VERSION
genpkgup -p -a $ARCH -v $VERSION > index.pkgup
gzip index.pkgup
mv index.pkgup.gz $PKGUP_DIR/$VERSION/$ARCH/
