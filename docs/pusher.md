Pusher
======

To start the pusher service you should use one of the commands available in the CLI.

## CLI

The configuration file contains the information about which apps will be enabled, as well as the required informations to actually send the pushes either to GCM or APNS. All variables in the configuration file can be overwritten by environment variables, for example, to overwrite the GCM apps the environment variable is: `PUSHER_GCM_APPS`.

There are some optional flags for both APNS and GCM:

```
--config: path to the config file (default "./config/default.yaml")
--debug/-d: debug mode switch (default is false, i.e., info level)
--json/-j: use json format for logging (default is false, logging is in text format)
--production/-p: production mode switch (default is false, development/sandbox environment is used)
```

### APNS

Example for running in production with default configuration and in debug mode:

```bash
❯ pusher apns -d -p
```

### GCM

Example for running in production with default configuration and in debug mode:

```bash
❯ pusher gcm -d -p
```

### Version

To print the current version of the lib simply run `pusher version`.

```bash
❯ pusher version
0.1.0
```

## Architecture

When the cli command is run, at first it configures either an APNSPusher or GCMPusher and then starts it.

The configuration step consists of:
- Configuring a Queue;
- Configuring a Message Handler;
- Configuring stats and feedback reporters specified in the configuration;

When a pusher is started it launches a series of goroutines:
- MessageHandler.HandleMessages
- MessageHandler.HandleResponses
- Queue.ConsumeLoop

### Queue

Queue is an interface from where the push notifications to be sent are consumed. The core of the queue is the ConsumeLoop function. When a message arrives in this queue it is sent to the MessagesChannel. For now the only queue that is supported is a Kafka consumer.

### Message Handler

The message handler is an interface witch has only two methods: HandleMessages and HandleResponses. HandleMessages listens to the MessagesChannel written by the Queue's ConsumeLoop. For each message that arrives in this channel it builds the APNSMessage or GCMMessage and sends it to the corresponding service. In the case of GCM it uses a XMPP connection and for APNS it is a HTTP2 connection. HandleResponses method receives the services feedbacks and process them.

### Stats Reporters

Stats reporters is an interface that reports stats for three cases:

- HandleNotificationSent: when a push notification is sent to APNS or GCM;
- HandleNotificationSuccess: when a successful feedback is received;
- HandleNotificationFailure: when a failure feedback is received;

For now the only reporter that is supported is Statsd. We are reporting only counters for the metrics above. In the case of failure we're also keeping track of the specific error.

### Feedback Reporters

A feedback reporter interface only implements a SendFeedback method that receives an feedback and sends it to the specified reporter. For now the only reporter that is supported is a Kafka producer.

### Invalid Token Handlers

An InvalidTokenHandler is an interface that implements a HandleToken method that is called when a failure feedback is received and the error indicates that the provided token is invalid. For now we have a handler implement that deletes this token from a PostgreSQL database containing user tokens.
