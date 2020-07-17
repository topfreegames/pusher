#!/bin/bash

set -xe

BUILD_PATH=`pwd`
LIBRDKAFKA_PATH=$HOME/.cache/librdkafka/$LIBRDKAFKA_VERSION

mkdir -p $HOME/.cache/librdkafka

if [ ! -d $LIBRDKAFKA_PATH ]; then
    git clone --depth 1 --branch "$LIBRDKAFKA_VERSION" https://github.com/edenhill/librdkafka.git $LIBRDKAFKA_PATH
    (
	cd $LIBRDKAFKA_PATH
	./configure
	make
    )
fi
(
    cd $LIBRDKAFKA_PATH
    git remote update
    UPTODATE='`git status -uno | grep \"Your branch is up-to-date\"`'
    if [ "$UPTODATE" == "" ]; then
	(
	    git reset --hard
	    git pull
	    ./configure
	    make
	)
    fi
    make install
    ldconfig
)

cd $BUILD_PATH
