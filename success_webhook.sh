#!/bin/bash

if [ "$LIBRDKAFKA_VERSION" != "master" ]; then
  VERSION=$(cat ./util/version.go | grep "var Version" | awk ' { print $4 } ' | sed s/\"//g)
  curl -X POST "$FARM_URL$VERSION.$TRAVIS_BUILD_NUMBER"
fi
