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

import (
	"fmt"
	"time"

	pg "gopkg.in/pg.v5"

	"github.com/spf13/viper"
)

// PGClient is the struct that connects to PostgreSQL
type PGClient struct {
	Config *viper.Viper
	DB     *pg.DB
}

// NewPGClient creates a new client
func NewPGClient(prefix string, config *viper.Viper) (*PGClient, error) {
	client := &PGClient{
		Config: config,
	}
	err := client.Connect(prefix)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// Connect to PG
func (c *PGClient) Connect(prefix string) error {
	user := c.Config.GetString(fmt.Sprintf("%s.user", prefix))
	pass := c.Config.GetString(fmt.Sprintf("%s.pass", prefix))
	host := c.Config.GetString(fmt.Sprintf("%s.host", prefix))
	db := c.Config.GetString(fmt.Sprintf("%s.database", prefix))
	port := c.Config.GetInt(fmt.Sprintf("%s.port", prefix))
	poolSize := c.Config.GetInt(fmt.Sprintf("%s.poolSize", prefix))
	maxRetries := c.Config.GetInt(fmt.Sprintf("%s.maxRetries", prefix))

	conn := pg.Connect(&pg.Options{
		Addr:       fmt.Sprintf("%s:%d", host, port),
		User:       user,
		Password:   pass,
		Database:   db,
		PoolSize:   poolSize,
		MaxRetries: maxRetries,
	})
	c.DB = conn

	return nil
}

// IsConnected determines if the client is connected to PG
func (c *PGClient) IsConnected() bool {
	res, err := c.DB.Exec("select 1")
	if err != nil {
		return false
	}
	return res.RowsReturned() == 1
}

// Close the connections to PG
func (c *PGClient) Close() error {
	err := c.DB.Close()
	if err != nil {
		return err
	}
	return nil
}

// WaitForConnection loops until kafka is connected
func (c *PGClient) WaitForConnection(timeout int) error {
	start := time.Now().Unix()
	for !c.IsConnected() || time.Now().Unix()-start > int64(timeout) {
		time.Sleep(10 * time.Millisecond)
	}

	if time.Now().Unix()-start > int64(timeout) {
		return fmt.Errorf("Timed out waiting for Zookeeper to connect.")
	}
	return nil
}

//Cleanup closes PG connection
func (c *PGClient) Cleanup() error {
	err := c.Close()
	if err != nil {
		return err
	}
	return nil
}
