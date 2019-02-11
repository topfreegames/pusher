# -*- coding: utf-8 -*-

#
# Copyright (c) 2016 TFG Co <backend@tfgco.com>
# Author: TFG Co <backend@tfgco.com>
# Permission is hereby granted, free of charge, to any person obtaining a copy of
# this software and associated documentation files (the "Software"), to deal in
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

import urllib
import urllib2
import json


def main():
    url = "https://auth.docker.io/token?service=registry.docker.io&scope=repository:tfgco/pusher:pull,push"
    response = urllib.urlopen(url)
    token = json.loads(response.read())['token']

    url = "https://registry-1.docker.io/v2/tfgco/pusher/tags/list"
    req = urllib2.Request(url, None, {
        "Authorization": "Bearer %s" % token,
    })
    response = urllib2.urlopen(req)
    tags = json.loads(response.read())
    last_tag = get_last_tag(tags['tags'])
    print last_tag


def get_tag_value(tag):
    if "latest" in tag:
        return 0
    for t in tag:
        if "rc" in t:
            return 0

    while len(tag) < 4:
        tag.append('0')

    total_value = 0
    for index, tag_part in enumerate(tag):
        power = pow(100, len(tag) - index)
        total_value += int(tag_part) * power

    return total_value


def get_last_tag(tags):
    return '.'.join(
        max(
            [(get_tag_value(tag), tag) for tag in [t.split('.') for t in tags]],
            key=lambda i: i[0]
        )[1]
    )


if __name__ == "__main__":
    main()
