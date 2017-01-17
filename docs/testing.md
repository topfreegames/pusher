Testing
=======

## Benchmark

Pusher can be bench tested using fake data. We provide a python script for generating these messages:

```bash
cd bench

python create_bench.py test.txt

```

This command will create a file `text.txt` containing several lines like this:

- GCM

```
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

```
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

### Send pushes using the fake data:

To send the push using the fake data you need to start `pusher` in the correct mode (apns or gcm) and then produce to the Kafka topic and broker it will be listening to:

```bash
cat test.txt | kafka-console-producer --topic <topic> --broker-list <broker>
```

## Automated tests

We're using Ginkgo and Gomega for testing our code. Since we're making extensive use of interfaces, external dependencies are mocked for all unit tests.

### Unit Tests
We'll try to keep testing coverage as high as possible. To run unit tests simply use:

```
make unit
```

To check the test coverage use:

```
make cover  # opens a html file
```

or

```
make test-coverage-func  # prints code coverage to the console
```

### Integration Tests

We also have a few integration tests that actually connect to pusher external dependencies. For these you'll need to have docker installed.

To run integration tests run:

```
make integration
```
