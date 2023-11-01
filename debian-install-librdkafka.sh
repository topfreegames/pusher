#!/bin/sh

set -xe

apt-get install -y gcc g++ python

rm -rf ./librdkafka

git clone --depth 1 --branch "v2.2.0" https://github.com/confluentinc/librdkafka.git
(
    cd librdkafka
    ./configure
    make
    make install
)
ldconfig
