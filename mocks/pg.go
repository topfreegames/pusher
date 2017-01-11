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

package mocks

import (
	"fmt"

	"gopkg.in/pg.v5/types"
)

//PGMock should be used for tests that need to connect to PG
type PGMock struct {
	Execs        [][]interface{}
	ExecOnes     [][]interface{}
	Queries      [][]interface{}
	Closed       bool
	RowsAffected int
	RowsReturned int
	Error        error
}

//NewPGMock creates a new instance
func NewPGMock(rowsAffected, rowsReturned int, errOrNil ...error) *PGMock {
	var err error
	if len(errOrNil) == 1 {
		err = errOrNil[0]
	}
	return &PGMock{
		Closed:       false,
		RowsAffected: rowsAffected,
		RowsReturned: rowsReturned,
		Error:        err,
	}
}

func (m *PGMock) getResult() *types.Result {
	return types.NewResult([]byte(fmt.Sprintf(" %d", m.RowsAffected)), m.RowsReturned)
}

//Close records that it is closed
func (m *PGMock) Close() error {
	m.Closed = true

	if m.Error != nil {
		return m.Error
	}

	return nil
}

//Exec stores executed params
func (m *PGMock) Exec(obj interface{}, params ...interface{}) (*types.Result, error) {
	op := []interface{}{
		obj, params,
	}
	m.Execs = append(m.Execs, op)

	if m.Error != nil {
		return nil, m.Error
	}

	result := m.getResult()
	return result, nil
}

//ExecOne stores executed params
func (m *PGMock) ExecOne(obj interface{}, params ...interface{}) (*types.Result, error) {
	op := []interface{}{
		obj, params,
	}
	m.ExecOnes = append(m.ExecOnes, op)

	if m.Error != nil {
		return nil, m.Error
	}

	result := m.getResult()
	return result, nil
}

//Query stores executed params
func (m *PGMock) Query(obj interface{}, query interface{}, params ...interface{}) (*types.Result, error) {
	op := []interface{}{
		obj, query, params,
	}
	m.Execs = append(m.Execs, op)

	if m.Error != nil {
		return nil, m.Error
	}

	result := m.getResult()
	return result, nil
}
