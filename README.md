Pusher
======

### Building

```
make build
```

### Dependencies
* Go 1.7
* [librdkafka](https://github.com/edenhill/librdkafka)

### TODO

- [ ] Do we need concurrency control e.g. max buffer for inflight messages?
- [ ] Only commit kafka offsets on success?
- [ ] Grab from kafka in batches?
- [ ] GCM staging support
- [ ] Handle all responses from APNS and GCM
