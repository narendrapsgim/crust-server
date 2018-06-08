#!/bin/bash
set -e
PROJECT=$(basename $(dirname $(readlink -f $0)))

if [ -d "vendor" ]; then
	docker run --rm -v $(pwd):/go/src/github.com/titpetric/$PROJECT -w /go/src/github.com/titpetric/$PROJECT -e GOOS=${OS} -e GOARCH=${ARCH} -e CGO_ENABLED=0 -e GOARM=7 titpetric/golang dep ensure
else
	docker run --rm -v $(pwd):/go/src/github.com/titpetric/$PROJECT -w /go/src/github.com/titpetric/$PROJECT -e GOOS=${OS} -e GOARCH=${ARCH} -e CGO_ENABLED=0 -e GOARM=7 titpetric/golang dep init
fi

NAMES=$(ls cmd/* -d | xargs -n1 basename)
for NAME in $NAMES; do
	OSES=${OSS:-"linux"}
	ARCHS=${ARCHS:-"amd64"}
	for ARCH in $ARCHS; do
		for OS in $OSES; do
			echo $OS $ARCH $NAME
			docker run --rm -v $(pwd):/go/src/github.com/titpetric/$PROJECT -w /go/src/github.com/titpetric/$PROJECT -e GOOS=${OS} -e GOARCH=${ARCH} -e CGO_ENABLED=0 -e GOARM=7 titpetric/golang go build -o build/${NAME}-${OS}-${ARCH} cmd/${NAME}/*.go
			if [ $? -eq 0 ]; then
				echo OK
			fi
			if [ "$OS" == "windows" ]; then
				mv build/${NAME}-${OS}-${ARCH} build/${NAME}-${OS}-${ARCH}.exe
			fi
		done
	done
done