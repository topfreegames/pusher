#!/bin/sh

set -xe

apt-get install -y gcc g++ python

rm -rf ./librdkafka

git clone --depth 1 --branch "v0.11.3" https://github.com/edenhill/librdkafka.git
(
    cd librdkafka
    ./configure
    make
    make install
)
ldconfig
