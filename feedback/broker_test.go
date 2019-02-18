/*
 * Copyright (c) 2019 TFG Co <backend@tfgco.com>
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

package feedback

import (
	"encoding/json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	uuid "github.com/satori/go.uuid"
	"github.com/sideshow/apns2"
	"github.com/spf13/viper"
	gcm "github.com/topfreegames/go-gcm"
	"github.com/topfreegames/pusher/structs"
	"github.com/topfreegames/pusher/testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("Broker", func() {
	var logger *logrus.Logger
	var hook *test.Hook
	var inChan chan QueueMessage
	var config *viper.Viper
	var err error

	configFile := "../config/test.yaml"

	BeforeEach(func() {
		logger, hook = test.NewNullLogger()

		config, err = util.NewViperWithConfigFile(configFile)
		Expect(err).NotTo(HaveOccurred())

		inChan = make(chan QueueMessage, 100)
	})

	Describe("[Unit]", func() {
		It("Should start and stop correctly", func() {
			broker, err := NewBroker(logger, config, inChan, nil)
			Expect(err).NotTo(HaveOccurred())

			broker.Start()

			close(inChan)
			broker.Stop()
			Eventually(func() []*logrus.Entry { return hook.Entries }).
				Should(testing.ContainLogMessage("stop processing Broker's in channel"))
		})

		Describe("APNS Feedback Messages", func() {
			Describe("Invalid Token", func() {
				deviceToken := "CO8NP5B4PP51YVZ7FDMZI8QBLVI5HCFJRDK3YDOCLTFC9QOOJXVC2NNR8OM2UG5Y"
				game := "boomforce"
				platform := "apns"
				var value []byte
				var kafkaMsg QueueMessage

				BeforeEach(func() {
					value, err = json.Marshal(&structs.ResponseWithMetadata{
						StatusCode:  400,
						ApnsID:      uuid.NewV4().String(),
						Reason:      apns2.ReasonUnregistered,
						DeviceToken: deviceToken,
					})
					Expect(err).NotTo(HaveOccurred())

					kafkaMsg = &KafkaMessage{
						Game:     game,
						Platform: platform,
						Value:    value,
					}
				})

				It("Should route an invalid token feedback", func() {
					broker, err := NewBroker(logger, config, inChan, nil)
					Expect(err).NotTo(HaveOccurred())

					broker.Start()

					inChan <- kafkaMsg
					tk := <-broker.InvalidTokenOutChan

					expTk := &InvalidToken{
						Token:    deviceToken,
						Game:     game,
						Platform: platform,
					}
					Expect(tk).To(Equal(expTk))

					broker.Stop()
					Expect(len(broker.InChan)).To(Equal(0))
					Expect(len(broker.InvalidTokenOutChan)).To(Equal(0))
				})
			})
		})

		Describe("GCM Feedback Messages", func() {
			Describe("Invalid Token", func() {
				deviceToken := "LZ4KXN4NWY72LIZCGWNGS2E6NLCGZZKFUH1R0EHQFG18SF4IXYUF7U0D539IIYIM2WP59YXFSBD9RK4WLFZFPVTP63PTRTI92LPUF1JYYNJUAP98UDHNB4ZYZBSNNFRF2DC34G6BJ721CA0VNKZL41QR"
				game := "boomforce"
				platform := "gcm"
				var value []byte
				var kafkaMsg QueueMessage

				BeforeEach(func() {
					value, err = json.Marshal(&gcm.CCSMessage{
						From:  deviceToken,
						Error: "DEVICE_UNREGISTERED",
					})
					Expect(err).NotTo(HaveOccurred())

					kafkaMsg = &KafkaMessage{
						Game:     game,
						Platform: platform,
						Value:    value,
					}
				})

				It("Should route an invalid token feedback from GCM", func() {
					broker, err := NewBroker(logger, config, inChan, nil)
					Expect(err).NotTo(HaveOccurred())

					broker.Start()

					inChan <- kafkaMsg
					tk := <-broker.InvalidTokenOutChan

					expTk := &InvalidToken{
						Token:    deviceToken,
						Game:     game,
						Platform: platform,
					}
					Expect(tk).To(Equal(expTk))

					broker.Stop()
					Expect(len(broker.InChan)).To(Equal(0))
					Expect(len(broker.InvalidTokenOutChan)).To(Equal(0))
				})
			})
		})
	})
})
