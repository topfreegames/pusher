Hosting Pusher
==============

## Docker

Running Pusher with docker is rather simple.

Pusher reads from Kafka the push notifications that should be sent. The container takes environment variables to specify this connection:

* `PUSHER_QUEUE_TOPICS` - List of Kafka topics;
* `PUSHER_QUEUE_BROKERS` - List of Kafka brokers;
* `PUSHER_QUEUE_GROUP` - Kafka consumer group;
* `PUSHER_QUEUE_SESSIONTIMEOUT` - Kafka session timeout;
* `PUSHER_QUEUE_OFFSETRESETSTRATEGY` - Kafka offset reset strategy;
* `PUSHER_QUEUE_HANDLEALLMESSAGESBEFOREEXITING` - Boolean indicating if shutdown should wait for all messages to be handled;

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

### Example command for running with Docker

```
    $ docker pull tfgco/pusher
    $ docker run -t --rm -e "PUSHER_PUSH_DB_HOST=<postgres host>" -e "PUSHER_PUSH_DB_HOST=<postgres port>" tfgco/pusher
```
