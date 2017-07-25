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

package pusher

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/interfaces"
)

type invalidTokenHandlerInitializer func(*viper.Viper, *logrus.Logger, interfaces.DB) (interfaces.InvalidTokenHandler, error)

// AvailableInvalidTokenHandlers contains functions to initialize all invalid token handlers
var AvailableInvalidTokenHandlers = map[string]invalidTokenHandlerInitializer{
	"pg": func(config *viper.Viper, logger *logrus.Logger, dbOrNil interfaces.DB) (interfaces.InvalidTokenHandler, error) {
		return extensions.NewTokenPG(config, logger, dbOrNil)
	},
}

func configureInvalidTokenHandlers(config *viper.Viper, logger *logrus.Logger, dbOrNil interfaces.DB) ([]interfaces.InvalidTokenHandler, error) {
	invalidTokenHandlers := []interfaces.InvalidTokenHandler{}
	invalidTokenHandlerNames := config.GetStringSlice("invalidToken.handlers")
	for _, invalidTokenHandlerName := range invalidTokenHandlerNames {
		invalidTokenHandlerFunc, ok := AvailableInvalidTokenHandlers[invalidTokenHandlerName]
		if !ok {
			return nil, fmt.Errorf("Failed to initialize %s. Invalid Token Handler not available.", invalidTokenHandlerName)
		}

		r, err := invalidTokenHandlerFunc(config, logger, dbOrNil)
		if err != nil {
			return nil, fmt.Errorf("Failed to initialize %s. %s", invalidTokenHandlerName, err.Error())
		}
		invalidTokenHandlers = append(invalidTokenHandlers, r)
	}

	return invalidTokenHandlers, nil
}
