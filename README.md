[![Build Status](https://travis-ci.org/topfreegames/pusher.svg?branch=master)](https://travis-ci.org/topfreegames/pusher)
[![Coverage Status](https://coveralls.io/repos/github/topfreegames/pusher/badge.svg?branch=master)](https://coveralls.io/github/topfreegames/pusher?branch=master)
[![Docs](https://readthedocs.org/projects/pusher/badge/?version=latest)](http://pusher.readthedocs.io/en/latest/)

Pusher
======

# What is Pusher?

Pusher is a fast push notification platform for APNS and GCM.

## Features

* **Multi-services** - Pusher supports both gcm and apns services, but plugging a new one shouldn't be difficult;
* **Stats reporters** - Reporting of sent, successful and failed notifications can be done easily with pluggable stats reporters. For now only StatsD is supported;
* **Feedbacks reporters** - Feedbacks from APNS and GCM can be easily sent to pluggable reporters. For now only Kafka producer is supported;
* **Handling of invalid tokens** - Tokens which receive an invalid token feedback can be easily handled by pluggable handlers. For now they are deleted from a PostgreSQL database;
* **Easy to deploy** - Pusher comes with containers already exported to docker hub for every single of our successful builds.

# Running

To start the pusher service you should use one of the commands available in the CLI.

The configuration file contains the information about which apps will be enabled, as well as the required informations to actually send the pushes either to GCM or APNS. All variables in the configuration file can be overwritten by environment variables, for example, to overwrite the GCM apps the environment variable is: `PUSHER_GCM_APPS`.

There are some optional flags for both APNS and GCM:

```
--config: path to the config file (default "./config/default.yaml")
--debug/-d: debug mode switch (default is false, i.e., info level)
--json/-j: use json format for logging (default is false, logging is in text format)
--production/-p: production mode switch (default is false, development/sandbox environment is used)
```

## APNS

Example for running in production with default configuration and in debug mode:

```bash
❯ pusher apns -d -p
```

## GCM

Example for running in production with default configuration and in debug mode:

```bash
❯ pusher gcm -d -p
```

## Version

To print the current version of the lib simply run `pusher version`.

```bash
❯ pusher version
0.1.0
```

## Available Environment variables

Pusher reads from Kafka the push notifications that should be sent. The container takes environment variables to specify this connection:

* `PUSHER_QUEUE_TOPICS` - List of Kafka topics, ex: `^push-[^-_]+_(apns|gcm)`
* `PUSHER_QUEUE_BROKERS` - List of Kafka brokers;
* `PUSHER_QUEUE_GROUP` - Kafka consumer group;
* `PUSHER_QUEUE_SESSIONTIMEOUT` - Kafka session timeout;
* `PUSHER_QUEUE_OFFSETRESETSTRATEGY` - Kafka offset reset strategy;
* `PUSHER_QUEUE_HANDLEALLMESSAGESBEFOREEXITING` - Boolean indicating if shutdown should wait for all messages to be handled;

Pusher gets the GCM or APNS keys info from environment variables:

* `PUSHER_GCM_APPS` - Comma separated APNS app names, ex: appname1,appname2;
* `PUSHER_APNS_APPS` - Comma separated GCM app names, ex: appname1,appname2;
* `PUSHER_APNS_CERTS_APPNAME` - App APNS certificate path
* `PUSHER_GCM_CERTS_APPNAME_APIKEY` - GCM App Api Key
* `PUSHER_GCM_CERTS_APPNAME_SENDERID` - GCM App SenderID

For feedbacks you must specify a list of reporters:

* `PUSHER_FEEDBACK_REPORTERS` - List of feedbacks reporters;

For each specified reporter you can set its configuration. For Kafka, it is as follows:

* `PUSHER_FEEDBACK_KAFKA_TOPICS` - List of Kafka topics;
* `PUSHER_FEEDBACK_KAFKA_BROKERS` - List of Kafka brokers;

The same logic is used for stats:

* `PUSHER_STATS_REPORTERS` - List of feedbacks reporters;

For a Statsd stats reporter, it is as follows:

* `PUSHER_STATS_STATSD_HOST` - Statsd host;
* `PUSHER_STATS_STATSD_PREFIX` - Prefix used in Statsd reported metrics;
* `PUSHER_STATS_STATSD_FLUSHINTERVALINMS` - Interval (in milliseconds) during which stats are aggregated before they are sent to the statsd server;

You can also specify invalid token handlers:

* `PUSHER_INVALIDTOKEN_HANDLERS` - List of invalid token handlers;

If Pusher needs to connect to a PostgreSQL database in order to delete invalid tokens the following environment variables must be specified:

* `PUSHER_INVALIDTOKEN_PG_USER` - User of the PostgreSQL Server to connect to;
* `PUSHER_INVALIDTOKEN_PG_PASS` - Password of the PostgreSQL Server to connect to;
* `PUSHER_INVALIDTOKEN_PG_HOST` - PostgreSQL host to connect to;
* `PUSHER_INVALIDTOKEN_PG_DATABASE` - PostgreSQL database to connect to;
* `PUSHER_INVALIDTOKEN_PG_PORT` - PostgreSQL port to connect to;
* `PUSHER_INVALIDTOKEN_PG_POOLSIZE` - PostgreSQL connection pool size;
* `PUSHER_INVALIDTOKEN_PG_MAXRETRIES` - PostgreSQL connection max retries;
* `PUSHER_INVALIDTOKEN_PG_CONNECTIONTIMEOUT` - Timeout for trying to establish connection;

Other than that, there are a couple more configurations you can pass using environment variables:

* `PUSHER_GRACEFULLSHUTDOWNTIMEOUT` - Pusher is exited gracefully but you should specify a timeout for termination in case it takes too long;


The APNS library we're using supports several concurrent workers.
* `PUSHER_APNS_CONCURRENTWORKERS` - Amount of concurrent workers;

The GCM library we're using requires that we specify a ping interval and timeout for the XMPP connection.
* `PUSHER_GCM_PINGINTERVAL` - Ping interval in seconds;
* `PUSHER_GCM_PINGTIMEOUT` - Ping timeout in seconds;

GCM supports at most 100 pending messages (see [Flow Control section](https://developers.google.com/cloud-messaging/ccs#flow) in GCM documentation).

* `PUSHER_GCM_MAXPENDINGMESSAGES` - Max pending messages;

If you wish Sentry integration simply set the following environment variable:

* `PUSHER_SENTRY_URL` - Sentry Client Key (DSN);


# Architecture

When the cli command is run, at first it configures either an APNSPusher or GCMPusher and then starts it.

The configuration step consists of:
- Configuring a Queue;
- Configuring a Message Handler;
- Configuring stats and feedback reporters specified in the configuration;

When a pusher is started it launches a series of goroutines:
- MessageHandler.HandleMessages
- MessageHandler.HandleResponses
- Queue.ConsumeLoop

## Queue

Queue is an interface from where the push notifications to be sent are consumed. The core of the queue is the ConsumeLoop function. When a message arrives in this queue it is sent to the MessagesChannel. For now the only queue that is supported is a Kafka consumer.

## Message Handler

The message handler is an interface witch has only two methods: HandleMessages and HandleResponses. HandleMessages listens to the MessagesChannel written by the Queue's ConsumeLoop. For each message that arrives in this channel it builds the APNSMessage or GCMMessage and sends it to the corresponding service. In the case of GCM it uses a XMPP connection and for APNS it is a HTTP2 connection. HandleResponses method receives the services feedbacks and process them.

## Stats Reporters

Stats reporters is an interface that reports stats for three cases:

- HandleNotificationSent: when a push notification is sent to APNS or GCM;
- HandleNotificationSuccess: when a successful feedback is received;
- HandleNotificationFailure: when a failure feedback is received;

For now the only reporter that is supported is Statsd. We are reporting only counters for the metrics above. In the case of failure we're also keeping track of the specific error.

## Feedback Reporters

A feedback reporter interface only implements a SendFeedback method that receives an feedback and sends it to the specified reporter. For now the only reporter that is supported is a Kafka producer.

## Invalid Token Handlers

An InvalidTokenHandler is an interface that implements a HandleToken method that is called when a failure feedback is received and the error indicates that the provided token is invalid. For now we have a handler implement that deletes this token from a PostgreSQL database containing user tokens.

# Development

Since the current Go version used by Pusher is 1.10, an obsolete version, it is recommended to use Docker to avoid installing unwanted (obsolete) tools on your machine. However, documentation has not been updated for a while and almost everything described down here relies on installing and running everything on your machine. See [Using Docker to avoid contaminating your local environment](#using-docker-to-avoid-contaminating-your-local-environment) for further details.

## Dependencies

* Go 1.10.
* Kafka >= 0.9.0 using [librdkafka](https://github.com/confluentinc/librdkafka).
* Postgres >= 9.5.
* StatsD.

## Setup

First, set your $GOPATH ([Go Lang](https://golang.org/doc/install)) env variable and add $GOPATH/bin to your $PATH

```bash
make setup
```

### Building

```bash
make build
```

### Sending pushes

#### APNS

Example for running in production with default configuration and in debug mode:

```bash
./bin/pusher apns -d -p
```

#### GCM

```bash
./bin/pusher gcm -d -p
```

## Linter

Run [golangci-lint](https://golangci-lint.run/) using:

```bash
make lint
```

## Testing

### Automated tests

We're using [Ginkgo](https://onsi.github.io/ginkgo) and [Gomega](https://onsi.github.io/gomega) for testing our code. Since we're making extensive use of interfaces, external dependencies are mocked for all unit tests.

#### Unit Tests
We'll try to keep testing coverage as high as possible. To run unit tests simply use:

```bash
make unit
```

To check the test coverage use:

```bash
make cover  # opens a html file
```

or

```bash
make test-coverage-func  # prints code coverage to the console
```

#### Integration Tests

We also have a few integration tests that actually connect to pusher external dependencies. For these you'll need to have docker installed.

To run integration tests run:

```
make integration
```

If you are running integration tests locally with the most recent librdkafka version (such as installed by brew) some of them will fail due to incompatible librdkafka version. 
Tests should work for librdkafka v0.11.5.

### Benchmark

#### Create fake push data

Pusher can be bench tested using fake data. We provide a python script for generating these messages:

```bash
cd bench

python create_bench.py test.txt

```

This command will create a file `text.txt` containing several lines like this:

- GCM

```json
{
  "to": "XNAJY2WCN7RDH6B5APHXTCM793X28IO7780AB51F0F8OV3ENWXOIR40JRF3K9416AD9K029NEE3XTA229NJC0Y6DHCBO13EE6IFO6VRF8FICJ317AC5I3N1FCSJ7KIVXMKZ088BJOVS3PPJUG9CWV1J2",
  "notification": {
    "title": "Come play!",
    "body": "Helena miss you! come play!"
  },
  "dry_run": true
}
```

Note: If you want to actually send the pushes you need to set `dry_run: false` (default is true).

- APNS

```json
{
  "DeviceToken":"H9CSRZHTAUPOZP1ZDLK46DN8L1DS4JFIUKHXE33K77QHQLHZ650TG66U49ZQGFZV",
  "Payload": {
    "aps": {
      "alert": "Helena miss you! come play!"
    }
  },
  "push_expiry":0
}
```

#### Send pushes using the fake data:

To send the push using the fake data you need to start `pusher` in the correct mode (apns or gcm) and then produce to the Kafka topic and broker it will be listening to:

```bash
cat test.txt | kafka-console-producer --topic push-game_gcm --broker-list localhost:9941
```

### Using Docker to avoid contaminating your local environment

It is recommended to use Docker to develop and test Pusher, since all dependencies can be considered obsolete. In [Makefile](Makefile) you will find some helpers for that:

* build-image-dev: build an image of Pusher using [Dockerfile.local](Dockerfile.local), which is similar to [Dockerfile](Dockerfile) except it will inject [Kafka CLI scripts](https://kafka.apache.org/quickstart) - and Java since it is a dependency - and [psql](https://www.postgresql.org/docs/9.5/app-psql.html) to ease the process of testing connections inside the container. [golangci-lint](https://golangci-lint.run/) is installed alongside. Also, Pusher code is not built like in the production image.
* build-container-dev: runs `make build` inside a container that uses the image built by `build-image-dev`.
* lint-container-dev: runs `golangci-lint run` inside a container that uses the image built by `build-image-dev`. Configure linter rules at [.golangci.yml](.golangci.yml).
* unit-test-container-dev: runs `make unit` inside a container that uses the image built by `build-image-dev`.
* integration-test-container-dev: runs `make-run-integration` inside a container that uses the image built by `build-image-dev` after setting up dependencies using `docker compose`.
