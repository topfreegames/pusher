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

FROM golang:1.23 AS dependencies

ENV LIBRDKAFKA_VERSION 2.2.0
ENV CPLUS_INCLUDE_PATH /usr/local/include
ENV LIBRARY_PATH /usr/local/lib
ENV LD_LIBRARY_PATH /usr/local/lib

WORKDIR /go/src/github.com/topfreegames/pusher

# RUN apk add --no-cache make git g++ bash python wget pkgconfig && \
#     wget -O /root/librdkafka-${LIBRDKAFKA_VERSION}.tar.gz https://github.com/confluentinc/librdkafka/archive/v${LIBRDKAFKA_VERSION}.tar.gz && \
#     tar -xzf /root/librdkafka-${LIBRDKAFKA_VERSION}.tar.gz -C /root && \
#     cd /root/librdkafka-${LIBRDKAFKA_VERSION} && \
#     ./configure && make && make install && make clean && ./configure --clean && \
#     mkdir -p /go/src/github.com/topfreegames/pusher

RUN apt update &&  \
    apt install -y wget && \
    wget -qO - https://packages.confluent.io/deb/7.5/archive.key | apt-key add - && \
    apt update && \
    apt install -y librdkafka-dev

RUN mkdir -p /go/src/github.com/topfreegames/pusher

ADD . /go/src/github.com/topfreegames/pusher

RUN make build
RUN mkdir /app && \
    mv /go/src/github.com/topfreegames/pusher/bin/pusher /app/pusher && \
    mv /go/src/github.com/topfreegames/pusher/config /app/config && \
    mv /go/src/github.com/topfreegames/pusher/tls /app/tls && \
    rm -r /go/src/github.com/topfreegames/pusher

WORKDIR /app

VOLUME /app/config
VOLUME /app/tls

CMD /app/pusher -h
