package main

import (
	"github.com/prawirdani/golang-restapi/config"
	"github.com/prawirdani/golang-restapi/internal/domain/auth"
	"github.com/prawirdani/golang-restapi/internal/domain/user"
	"github.com/prawirdani/golang-restapi/internal/infrastructure/messaging/rabbitmq"
	"github.com/prawirdani/golang-restapi/internal/infrastructure/repository/postgres"
	"github.com/prawirdani/golang-restapi/internal/infrastructure/storage/r2"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Services struct {
	UserService *user.Service
	AuthService *auth.Service
}

// Container holds all application dependencies
type Container struct {
	Config   *config.Config
	Services *Services
}

// NewContainer initializes all dependencies
func NewContainer(
	cfg *config.Config,
	pg *postgres.DB,
	rmqconn *amqp.Connection,
) (*Container, error) {
	r2Storage, err := r2.New(r2.Config{
		BucketURL:       cfg.R2.BucketURL,
		BucketName:      cfg.R2.Bucket,
		AccountID:       cfg.R2.AccountID,
		AccessKeyID:     cfg.R2.AccessKeyID,
		AccessKeySecret: cfg.R2.AccessKeySecret,
	})
	if err != nil {
		return nil, err
	}

	// Repos init
	userRepo := postgres.NewUserRepository(pg)
	authRepo := postgres.NewAuthRepository(pg)

	// Setup Services
	userService := user.NewService(pg, userRepo, r2Storage)

	authMessagePublisher := rabbitmq.NewAuthMessagePublisher(rmqconn)
	authSvc := auth.NewService(
		cfg.Auth,
		pg,
		userRepo,
		authRepo,
		authMessagePublisher,
	)

	c := &Container{
		Config: cfg,
		Services: &Services{
			UserService: userService,
			AuthService: authSvc,
		},
	}

	return c, nil
}
