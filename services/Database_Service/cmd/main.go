package main

import (
	"fmt"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
	"log"
	"os"
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
		Password: os.Getenv("DB_PASSWORD"),
		SSLMode:  viper.GetString("db.sslmode"),
	})
	if err != nil {
		log.Fatalf("error, initialization dbs, %s", err.Error())
	}
	defer db.Close()

	//user := models.User{
	//	Username:     "test3",
	//	Email:        "test3@gmail.com",
	//	FirstName:    "User1",
	//	LastName:     "User1_1",
	//	PasswordHash: "test1",
	//	IsActive:     true,
	//}
	//
	//_ = repository.PostUser(db, user)

	user, _ := repository.GetUsers(db)
	fmt.Println(err)
	fmt.Println(user)

}

func initConfig() error {
	viper.AddConfigPath("configs")
	viper.SetConfigName("config")
	viper.SetConfigName("db")
	return viper.ReadInConfig()
}
