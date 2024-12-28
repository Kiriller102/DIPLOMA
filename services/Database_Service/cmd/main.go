package main

import (
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
	"log"
	"os"
	"project/services/Database_Service/models"
	"project/services/Database_Service/repository"
)

func main() {
	if err := initConfig(); err != nil {
		log.Fatalf("error, initialization configs, %s", err.Error())
	}

	if err := godotenv.Load("configs/secrets.env"); err != nil {
		log.Fatalf("error loading env variables, %s", err.Error())
	}

	db, err := repository.NewPostgresDB(repository.Config{
		Host:     viper.GetString("db.host"),
		Port:     viper.GetString("db.port"),
		Username: viper.GetString("db.user"),
		DBName:   viper.GetString("db.name"),
		SSLMode:  viper.GetString("db.sslmode"),
		Password: os.Getenv("DB_PASSWORD"),
	})
	if err != nil {
		log.Fatalf("error, initialization db, %s", err.Error())
	}

	user := models.User{
		Username:     "test2",
		Email:        "test2@gmail.com",
		PasswordHash: "test1",
		IsActive:     true,
	}

	_ = repository.PostUser(db, user)
}

func initConfig() error {
	viper.AddConfigPath("configs")
	viper.SetConfigName("config")
	viper.SetConfigName("db")
	return viper.ReadInConfig()
}
