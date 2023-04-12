#!/bin/bash
set -e

VERSION="1.4.6"
PROTECTED_MODE="no"

export GO15VENDOREXPERIMENT=1

OD="$(pwd)"
WD=$OD

package() {
	echo Packaging pigo for armv6l ...
	rm -rf packages/
	build_dir=proglove-pigo
	output_file=proglove-pigo-linux-armv6l.tar.gz
	mkdir -p packages/$build_dir
	cd packages
	GOOS=linux GOARCH=arm GOARM=6 go build -ldflags "-X main.Version=$VERSION" -o $build_dir/proglove-pigo ../cmd/pigo/*.go
	cp ../cascade/facefinder $build_dir/cascade_facefinder
	tar -zcf proglove-pigo.tar.gz $build_dir
}

if [ "$1" == "package" ]; then
	package
	exit
fi
