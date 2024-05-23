package config

import (
	"encoding/json"
	"fmt"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/util"
	"reflect"
	"strings"
)

type (
	// Config is the struct that holds all the configuration for the Pusher.
	Config struct {
		GCM                     GCM
		Apns                    Apns
		Queue                   Kafka
		GracefulShutdownTimeout int
	}

	Kafka struct {
		Brokers string
	}

	GCM struct {
		Apps               string
		PingInterval       int
		PingTimeout        int
		MaxPendingMessages int
		LogStatsInterval   int
	}

	Firebase struct {
		ConcurrentWorkers int
	}

	Apns struct {
		Apps  string
		Certs map[string]Cert
	}

	Cert struct {
		AuthKeyPath string
		KeyID       string
		TeamID      string
		Topic       string
	}
)

// NewConfigAndViper returns a new Config object and the corresponding viper instance.
func NewConfigAndViper(configFile string) (*Config, *viper.Viper, error) {
	v, err := util.NewViperWithConfigFile(configFile)
	if err != nil {
		return nil, nil, err
	}

	if err := v.ReadInConfig(); err != nil {
		return nil, nil, fmt.Errorf("error reading config file from %s: %s", configFile, err)
	}

	config := &Config{}
	if err := v.Unmarshal(config, decodeHookFunc()); err != nil {
		return nil, nil, fmt.Errorf("error unmarshalling config: %s", err)
	}

	return config, v, nil
}

func (c *Config) GetGcmAppsArray() []string {
	arr := strings.Split(c.GCM.Apps, ",")
	res := make([]string, 0, len(arr))
	for _, a := range arr {
		res = append(res, strings.TrimSpace(a))
	}

	return res
}

func (c *Config) GetApnsAppsArray() []string {
	arr := strings.Split(c.Apns.Apps, ",")
	res := make([]string, 0, len(arr))
	for _, a := range arr {
		res = append(res, strings.TrimSpace(a))
	}

	return res
}

func decodeHookFunc() viper.DecoderConfigOption {
	hooks := mapstructure.ComposeDecodeHookFunc(
		StringToMapStringHookFunc(),
	)
	return viper.DecodeHook(hooks)
}

func StringToMapStringHookFunc() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{},
	) (interface{}, error) {
		if f.Kind() != reflect.String || t.Kind() != reflect.Map {
			return data, nil
		}

		if t.Key().Kind() != reflect.String || t.Elem().Kind() != reflect.String {
			return data, nil
		}

		raw := data.(string)
		if raw == "" {
			return map[string]string{}, nil
		}

		m := map[string]string{}
		err := json.Unmarshal([]byte(raw), &m)
		return m, err
	}
}
