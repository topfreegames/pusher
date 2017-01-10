#!/bin/bash

#
# Copyright (c) 2016 TFG Co <backend@tfgco.com>
# Author: TFG Co <backend@tfgco.com>
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal in
# the Software without restriction, including without limitation the rights to
# use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
# the Software, and to permit persons to whom the Software is furnished to do so,
# subject to the following conditions:
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
# FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
# COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
# IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
# CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
#

if [ "$TRAVIS_BRANCH" != "master" ]; then
    exit 0
fi

VERSION=$(cat ./pusher/version.go | grep "var Version" | awk ' { print $4 } ' | sed s/\"//g)

docker login -u="$DOCKER_USERNAME" -p="$DOCKER_PASSWORD"

docker build -t pusher .
docker tag pusher:latest tfgco/pusher:$VERSION.$TRAVIS_BUILD_NUMBER
docker tag pusher:latest tfgco/pusher:$VERSION
docker tag pusher:latest tfgco/pusher:latest
docker push tfgco/pusher:$VERSION.$TRAVIS_BUILD_NUMBER
docker push tfgco/pusher:$VERSION
docker push tfgco/pusher:latest

DOCKERHUB_LATEST=$(python get_latest_tag.py)

if [ "$DOCKERHUB_LATEST" != "$VERSION.$TRAVIS_BUILD_NUMBER" ]; then
    echo "Last version is not in docker hub!"
    echo "docker hub: $DOCKERHUB_LATEST, expected: $VERSION.$TRAVIS_BUILD_NUMBER"
    exit 1
fi
