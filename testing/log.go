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

package testing

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/onsi/gomega/types"
)

//ContainLogMessage validates that the specified log message exists in the given entries
func ContainLogMessage(expected string) types.GomegaMatcher {
	return &containLogMessageMatcher{
		expected: expected,
	}
}

type containLogMessageMatcher struct {
	expected string
}

func (matcher *containLogMessageMatcher) Match(actual interface{}) (success bool, err error) {
	entries := actual.([]*logrus.Entry)
	for _, entry := range entries {
		if entry.Message == matcher.expected {
			return true, nil
		}
	}
	return false, nil
}

func (matcher *containLogMessageMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%#v\nto contain log message \n\t%#v", actual, matcher.expected)
}

func (matcher *containLogMessageMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%#v\nnot to contain log message \n\t%#v", actual, matcher.expected)
}
