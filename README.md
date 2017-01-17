[![Build Status](https://travis-ci.org/topfreegames/pusher.svg?branch=master)](https://travis-ci.org/topfreegames/pusher)
[![Coverage Status](https://coveralls.io/repos/github/topfreegames/pusher/badge.svg?branch=master)](https://coveralls.io/github/topfreegames/pusher?branch=master)

Pusher
======

### Setup

```
make setup
```

### Building

```
make build
```

### Dependencies
* Go 1.7
* Kafka >= 0.9.0
* [librdkafka](https://github.com/edenhill/librdkafka)

```
brew install go
brew install librdkafka
```

### Sending pushes

#### APNS

```
./bin/pusher apns --certificate <path-to-unified-certificate>/unified.pem --app <app-name> -d -p
```

#### GCM

```
./bin/pusher gcm --apiKey <api-key> --senderId <sender-id> --app <app-name> -d -p
```

### Benchmark

#### Create fake push data

```
cd bench

python create_bench.py test.txt

```

text.txt will contain several lines like this:

- GCM

```
{"to": "XNAJY2WCN7RDH6B5APHXTCM793X28IO7780AB51F0F8OV3ENWXOIR40JRF3K9416AD9K029NEE3XTA229NJC0Y6DHCBO13EE6IFO6VRF8FICJ317AC5I3N1FCSJ7KIVXMKZ088BJOVS3PPJUG9CWV1J2", "notification": {"title": "Come play!", "body": "Helena miss you! come play!"}, "dry_run": true}
```

If you want to actually send the pushes you need to set `dry_run: false` (default is true).

- APNS

```
{"DeviceToken":"H9CSRZHTAUPOZP1ZDLK46DN8L1DS4JFIUKHXE33K77QHQLHZ650TG66U49ZQGFZV","Payload":{"aps":{"alert":"Helena miss you! come play!"}},"push_expiry":0}
```

#### Send pushes using the fake data:

cat test.txt | kafka-console-producer --topic com.games.test --broker-list localhost:9941


### TODO

- [x] Handle all responses from APNS and GCM
- [x] Test Everything
- [x] GCM staging support (send PR to https://github.com/google/go-gcm ?)
- [x] Graceful shutdown should empty channels before dying (the signal is already being caught)
- [x] Recover when connection to GCM or APNS is lost
- [x] Define kafka offset commit strategy (auto? Manual? One by one? In batches?)
- [x] Grab from kafka in batches? (I think it already does that)
- [x] Logging and stats
- [x] Send feedbacks to another kafka queue including metadata
- [x] Support metadata in incoming messages and include them in the feedback sent to the other queue
- [x] Do we need concurrency control e.g. max buffer for inflight messages, I think so, https://github.com/google/go-gcm/blob/master/gcm.go#L373 ?
- [x] Auto recovery when connection to kafka is lost (I think it already does, we only need to check for how much time it will try to recover)
- [x] Fix TODOs
- [x] Improve code coverage and make sure unit tests don't have external dependencies
- [x] Verify string concats specially when building queries (SQL injection susceptible)
- [x] Avoid replication of common gcm and apns code
- [ ] README with dev and deployment instructions
- [ ] Documentation
- [ ] Apple JWT tokens instead of certificates https://developer.apple.com/library/content/documentation/NetworkingInternet/Conceptual/RemoteNotificationsPG/APNSOverview.html#//apple_ref/doc/uid/TP40008194-CH8-SW1
- [ ] Retry pushes depending on the failure?
- [ ] Threads for gcm sender? (For each sender ID, GCM allows 1000 connections in parallel.: https://developers.google.com/cloud-messaging/ccs)
- [ ] Support delivery receipts ? (https://developers.google.com/cloud-messaging/ccs)
