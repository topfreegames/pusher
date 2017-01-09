#!/bin/sh

set -xe

git clone --depth 1 --branch "$LIBRDKAFKA_VERSION" https://github.com/edenhill/librdkafka.git
(
    cd librdkafka
    ./configure
    make
    sudo make install
)
sudo ldconfig

make test
