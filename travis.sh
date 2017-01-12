#!/bin/sh

set -xe

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
cd $LIBRDKAFKA_PATH
git fetch -a
git reset --hard
git pull
sudo make install
sudo ldconfig

sleep 3
make test
