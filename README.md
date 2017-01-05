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
./bin/pusher apns --certificate <path-to-unified-certificate>/unified.pem --environment production --app <app-name> -d
```

#### GCM

```
./bin/pusher gcm --apiKey <api-key> --senderId <sender-id> --app <app-name> -d
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

cat test.txt | kafka-console-producer --topic com.games.teste --broker-list localhost:9941


### TODO

- [x] Handle all responses from APNS and GCM
- [x] Test Everything
- [ ] GCM staging support (send PR to https://github.com/google/go-gcm ?)
- [ ] Logging and stats
- [ ] Auto recovery when connection to kafka is lost (I think it already does, we only need to check for how much time it will try to recover)
- [ ] Graceful shutdown should empty channels before dying (the signal is already being caught)
- [ ] Recover when connection to GCM or APNS is lost
- [ ] Send successfuly sent messages somewhere (kafka? psql?), including metadata to identify the game, push type, user id, etc. (extension bad_token_handler with a method Handle(chan <-token) that is easy to turn on/off and replace)
- [ ] Send logs of deleted tokens somewhere (kafka? psql?), including metadata to identify the game, push type, user id, when the token was created etc. (extension successful_message_handler with a method Handle(chan <-token) that is easy to turn on/off and replace)
- [ ] Report stats to graphite including deleted token stats
- [ ] Do we need concurrency control e.g. max buffer for inflight messages, I think so, https://github.com/google/go-gcm/blob/master/gcm.go#L373 ?
- [ ] Define kafka offset commit strategy (auto? Manual? One by one? In batches?)
- [ ] Grab from kafka in batches? (I think it already does that)
- [ ] README with dev and deployment instructions
- [ ] Apple JWT tokens instead of certificates https://developer.apple.com/library/content/documentation/NetworkingInternet/Conceptual/RemoteNotificationsPG/APNSOverview.html#//apple_ref/doc/uid/TP40008194-CH8-SW1
- [ ] Fix TODOs
