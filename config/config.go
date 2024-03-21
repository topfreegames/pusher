package config

import (
	"fmt"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/util"
	"strings"
)

type (
	// Config is the struct that holds all the configuration for the Pusher.
	Config struct {
		GCM                     GCM
		GracefulShutdownTimeout int
	}

	GCM struct {
		Apps                string
		PingInterval        int
		PingTimeout         int
		MaxPendingMessages  int
		LogStatsInterval    int
		FirebaseCredentials map[string]string
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
	if err := v.Unmarshal(config); err != nil {
		return nil, nil, fmt.Errorf("error unmarshalling config: %s", err)
	}

	return config, v, nil
}

func (c *Config) GetAppsArray() []string {
	arr := strings.Split(c.GCM.Apps, ",")
	res := make([]string, 0, len(arr))
	for _, a := range arr {
		res = append(res, strings.TrimSpace(a))
	}

	return res
}
