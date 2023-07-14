package main

import (
	"fmt"
	"log"

	"github.com/spf13/viper"
)

type Configuration struct {
	SystemdEnabled bool
	ReceiverUnit   string
	ReceiverPath   string
	ListenPort     uint
}

/*  Each item takes precedence over the item below it:
    explicit call to Set
    flag
    env
    config
    key/value store
    default
*/

func ReadConfig() (*Configuration, error) {

	if runningSystemd() {
		viper.SetDefault("SystemdEnabled", true)
	}

	viper.SetDefault("ReceiverUnit", "haproxy")
	viper.SetDefault("ListenPort", 443)

	viper.SetConfigName("habroker")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/etc/")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Println("Missing configuration file, using default values!")
		} else {
			return nil, fmt.Errorf("error reading configuration file: %w", err)
		}
	}

	var config Configuration
	err = viper.Unmarshal(&config)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling configuration: %w", err)
	}

	return &config, nil
}
