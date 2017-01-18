#!/bin/bash
kafka-topics --create --topic push-$1_gcm-single --partitions 10 --replication-factor 2 --zookeeper $ZOOKEEPER
kafka-topics --create --topic push-$1_gcm-massive --partitions 10 --replication-factor 2 --zookeeper $ZOOKEEPER
kafka-topics --create --topic push-$1_gcm-feedbacks --partitions 10 --replication-factor 2 --zookeeper $ZOOKEEPER
kafka-topics --create --topic push-$1_apns-single --partitions 10 --replication-factor 2 --zookeeper $ZOOKEEPER
kafka-topics --create --topic push-$1_apns-massive --partitions 10 --replication-factor 2 --zookeeper $ZOOKEEPER
kafka-topics --create --topic push-$1_apns-feedbacks --partitions 10 --replication-factor 2 --zookeeper $ZOOKEEPER
