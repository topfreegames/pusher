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
* [librdkafka](https://github.com/edenhill/librdkafka)

```
brew install go
brew install librdkafka
```

### TODO

- [ ] Do we need concurrency control e.g. max buffer for inflight messages?
- [ ] Only commit kafka offsets on success?
- [ ] Grab from kafka in batches? (max inflight messages)
- [ ] GCM staging support
- [ ] Handle all responses from APNS and GCM
- [ ] Test Everything
- [ ] Logging and stats
- [ ] Auto recovery when connection to kafka is lost
- [ ] Graceful shutdown should empty channels before dying
- [ ] Report stats to graphite including deleted token stats
- [ ] README with dev and deployment instructions
- [ ] Send successfuly sent messages to another kafka queue, including metadata to identify the game, push type, user id, etc.
- [ ] Send logs of deleted tokens to another kafka queue, including metadata to identify the game, push type, user id, when the token was created etc.
