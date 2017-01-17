Overview
========

What is Pusher? Pusher is a fast push notification platform for APNS and GCM.

## Features

* **Multi-services** - Pusher supports both gcm and apns services, but plugging a new one shouldn't be difficult;
* **Stats reporters** - Reporting of sent, successful and failed notifications can be done easily with pluggable stats reporters. For now only StatsD is supported;
* **Feedbacks reporters** - Feedbacks from APNS and GCM can be easily sent to pluggable reporters. For now only Kafka producer is supported;
* **Removal of invalid tokens** - Tokens which receive an invalid token feedback are automatically deleted from the database;
* **Easy to deploy** - Pusher comes with containers already exported to docker hub for every single of our successful builds.

## Architecture

Pusher is based on some premises:
- You have a PostgreSQL Database with tables containing the device token registered in apns or gcm service;

## The Stack

For the devs out there, our code is in Go, but more specifically:

* Kafka >= 0.9.0 using [librdkafka](https://github.com/edenhill/librdkafka);
* Database - Postgres >= 9.5;
* StatsD;

## Who's Using it

Well, right now, only us at TFG Co, are using it, but it would be great to get a community around the project. Hope to hear from you guys soon!

## How To Contribute?

Just the usual: Fork, Hack, Pull Request. Rinse and Repeat. Also don't forget to include tests and docs (we are very fond of both).
