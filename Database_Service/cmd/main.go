package main

import (
	"github.com/spf13/viper"
	"log"
)

func main() {
	if err := initConfig(); err != nil {
		log.Fatalf("error, initialization configs, %s", err.Error())
	}
}

func initConfig() error {
	viper.AddConfigPath("Database_service/configs")
	viper.SetConfigName("config")
	return viper.ReadInConfig()
}
