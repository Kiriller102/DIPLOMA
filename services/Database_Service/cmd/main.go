package main

import (
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
	"log"
	"os"
	"project/api/grpc"
	"project/api/rest"
	"project/api/rest/controllers"
	"project/repository"
	"project/services"
	"sync"
)

func main() {
	if err := godotenv.Load("configs/secrets.env"); err != nil {
		log.Fatalf("error loading env variables, %s", err.Error())
	}

	if err := initConfig(); err != nil {
		log.Fatalf("error, initialization configs, %s", err.Error())
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

	userService, err := services.NewUserService(db)
	if err != nil {
		log.Fatalf("error, initialization users, %s", err.Error())
		return
	}
	messageService, err := services.NewMessageService(db)
	if err != nil {
		log.Fatalf("error, initialization messages, %s", err.Error())
		return
	}
	chatService, err := services.NewChatService(db)
	if err != nil {
		log.Fatalf("error, initialization chats, %s", err.Error())
		return
	}
	chatParticipantService, err := services.NewChatParticipantService(db)
	if err != nil {
		log.Fatalf("error, initialization chats participants, %s", err.Error())
		return
	}

	userController := controllers.NewUserController(userService)
	messageController := controllers.NewMessageController(messageService)
	chatController := controllers.NewChatController(chatService)
	chatParticipantController := controllers.NewChatParticipantController(chatParticipantService)

	httpServer := rest.NewHTTPServer(userController, messageController, chatController, chatParticipantController)
	gRPCServer := grpc.NewGRPCServer("5050")
	gRPCServer.RegisterServices(userService, messageService, chatService, chatParticipantService)

	var wg sync.WaitGroup

	wg.Add(1)
	go httpServer.Run(&wg)

	wg.Add(1)
	go gRPCServer.Run(&wg)

	wg.Wait()
}

func initConfig() error {
	viper.AddConfigPath("configs")
	viper.SetConfigName("config")
	if err := viper.MergeInConfig(); err != nil {
		log.Fatalf("error loading configs, %s", err.Error())
		return err
	}
	viper.SetConfigName("db")
	return viper.MergeInConfig()
}
