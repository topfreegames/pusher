#!/bin/bash

if [ "$LIBRDKAFKA_VERSION" != "master" ]; then
  VERSION=$(cat ./api/version.go | grep "var VERSION" | awk ' { print $4 } ' | sed s/\"//g)
  COMMIT=$(git rev-parse --short HEAD)

  curl -X POST "$FARM_URL$TRAVIS_BUILD_NUMBER-v$VERSION-$COMMIT"
fi
