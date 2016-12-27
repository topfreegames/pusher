/*
 * Copyright (c) 2016 TFG Co <backend@tfgco.com>
 * Author: TFG Co <backend@tfgco.com>
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy of
 * this software and associated documentation files (the "Software"), to deal in
 * the Software without restriction, including without limitation the rights to
 * use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
 * the Software, and to permit persons to whom the Software is furnished to do so,
 * subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
 * FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
 * COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
 * IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
 * CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */

package extensions

import "github.com/confluentinc/confluent-kafka-go/kafka"

// Queue for getting push requests
type Queue struct {
	Topic               string
	Brokers             []string
	ConsumerGroup       string
	SessionTimeout      int
	OffsetResetStrategy string
	Consumer            *kafka.Consumer
	run                 bool
	Logger              logrus.Logger
	ConfigFile          string
	Config              *viper.Config
}

// NewQueue for creating a new Queue instance
func NewQueue(configFile string, logger *logrus.Logger) *Queue {
	q := &Queue{
		ConfigFile: configFile,
		Logger:     logger,
	}
	q.configure()
	return q
}

func (q *Queue) loadConfigurationDefaults() {
	q.Config.SetDefault("queue.topics", &string["com.games.teste"])
	q.Config.SetDefault("queue.brokers", &string["localhost:9092"])
	q.Config.SetDefault("queue.group", "teste")
	q.Config.SetDefault("queue.sessionTimeout", 6000)
	q.Config.SetDefault("queue.offsetResetStrategy", "earliest")
}

func (q *Queue) configure() {
	q.Config = viper.New()
	q.Config.SetConfigFile(q.ConfigFile)
	q.Config.SetEnvPrefix("pusher")
	q.Config.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	q.Config.AutomaticEnv()
	q.loadConfigurationDefaults()
	err := viper.ReadInConfig()
	if err != nil {
		q.Logger.Panicf("Fatal error config file: %s \n", err)
	}
	q.run = true
	q.OffsetResetStrategy = q.Config.GetString("queue.offsetResetStrategy")
	q.Brokers = q.Config.GetStringSlice("queue.brokers")
	q.ConsumerGroup = q.Config.GetString("queue.group")
	q.SessionTimeout = q.Config.GetInt("queue.sessionTimeout")
	q.Topics = q.Config.GetStringSlice("queue.topics")
	q.configureConsumer()
}

func (q *Queue) configureConsumer() {
	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":               q.Brokers,
		"group.id":                        q.ConsumerGroup,
		"session.timeout.ms":              q.SessionTimeout,
		"go.events.channel.enable":        true,
		"go.application.rebalance.enable": true,
		"default.topic.config":            kafka.ConfigMap{"auto.offset.reset": q.OffsetResetStrategy}})
	err := c.SubscribeTopics(q.topics, nil)
	if err != nil {
		q.Logger.PanicF("Error subscribing to topics %s\n%s", q.topics, err.Error())
	}
	q.Consumer = c
}

// ConsumeLoop consume messages from the queue and put in messages to send channel
func (q *Queue) ConsumeLoop() {
	for q.run == true {
		select {
		case sig := <-sigchan:
			q.Logger.Warnf("Caught signal %v: terminating\n", sig)
			run = false
		case ev := <-c.Events():
			switch e := ev.(type) {
			case kafka.AssignedPartitions:
				q.Logger.Infof(os.Stderr, "%% %v\n", e)
				c.Assign(e.Partitions)
			case kafka.RevokedPartitions:
				q.Logger.Warnf(os.Stderr, "%% %v\n", e)
				c.Unassign()
			case *kafka.Message:
				q.Logger.Debugf("%% Message on %s:\n%s\n",
					e.TopicPartition, string(e.Value))
			case kafka.PartitionEOF:
				q.Logger.Infof("%% Reached %v\n", e)
			case kafka.Error:
				q.Logger.Errorf(os.Stderr, "%% Error: %v\n", e)
				//TODO ver isso
				q.run = false
			}
		}
	}
}
