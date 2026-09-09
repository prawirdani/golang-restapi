package main

import (
	"github.com/prawirdani/golang-restapi/config"
	"github.com/prawirdani/golang-restapi/internal/auth"
	"github.com/prawirdani/golang-restapi/internal/infrastructure/postgres"
	"github.com/prawirdani/golang-restapi/internal/infrastructure/r2"
	redisInfra "github.com/prawirdani/golang-restapi/internal/infrastructure/redis"
	"github.com/prawirdani/golang-restapi/internal/rbac"
	"github.com/prawirdani/golang-restapi/internal/user"
	"github.com/redis/go-redis/v9"
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
	rdb *redis.Client,
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

	redisThrottler := redisInfra.NewRedisThrottler(rdb)

	// Repos init
	userRepo := postgres.NewUserRepository(pg)
	authRepo := postgres.NewAuthRepository(pg)
	auditRecorder := postgres.NewAuditRepository(pg)

	authorizer := rbac.NewAuthorizer()

	// Setup Services
	userService := user.NewService(pg, userRepo, r2Storage, authorizer, auditRecorder)

	authEventProducer := redisInfra.NewAuthEventProducer(rdb)
	authSvc := auth.NewService(
		cfg.Auth,
		pg,
		userRepo,
		authRepo,
		authorizer,
		authEventProducer,
		redisThrottler,
		auditRecorder,
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
