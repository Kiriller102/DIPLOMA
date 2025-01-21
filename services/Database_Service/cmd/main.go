package main

import (
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
	"log"
	"os"
	"project/api/rest"
	"project/repository"
	"project/services"
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
	/*
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

		//messageServ := services.NewMessageService(repository.MessageRepository(), db)
		userServ, err := services.NewUserService(db)
		if err != nil {
			log.Fatalf("error, initialization users, %s", err.Error())
		}

		users, err := userServ.GetAllUsers()

		r := gin.Default()

		r.GET("/ping", func(c *gin.Context) {
			c.JSON(200, users)
		})

		r.Run()*/
	userService, err := services.NewUserService(db)
	if err != nil {
		log.Fatalf("error, initialization users, %s", err.Error())
		return
	}
	userController := rest.NewUserController(userService)
	//if err != nil {
	//	log.Fatalf("error, initialization users, %s", err.Error())
	//	return
	//}

	router := gin.Default()

	userRoutes := router.Group("/users")
	{
		userRoutes.POST("", userController.CreateUser)
		userRoutes.GET("", userController.GetUser)
		userRoutes.GET("/all", userController.GetUsers)
		userRoutes.PUT("/:id", userController.UpdateUser)
		userRoutes.DELETE("/:id", userController.DeleteUser)
	}

	router.Run(":8080")
}

func initConfig() error {
	viper.AddConfigPath("configs")
	viper.SetConfigName("config")
	viper.SetConfigName("db")
	return viper.ReadInConfig()
}
