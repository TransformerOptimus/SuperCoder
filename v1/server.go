package main

import (
	"ai-developer/app/client"
	gitness_git_provider "ai-developer/app/client/git_provider"
	"ai-developer/app/client/workspace"
	"ai-developer/app/config"
	"ai-developer/app/constants"
	"ai-developer/app/controllers"
	"ai-developer/app/gateways"
	"ai-developer/app/middleware"
	"ai-developer/app/models"
	"ai-developer/app/monitoring"
	"ai-developer/app/repositories"
	"ai-developer/app/services"
	"ai-developer/app/services/auth"
	"ai-developer/app/services/filestore"
	"ai-developer/app/services/filestore/impl"
	"ai-developer/app/services/git_providers"
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/session"
	"log"
	"net/http"
	"time"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	socketio "github.com/googollee/go-socket.io"
	"github.com/hibiken/asynq"
	"github.com/knadh/koanf/v2"
	"github.com/newrelic/go-agent/v3/newrelic"
	"go.uber.org/dig"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func main() {

	config.InitLogger()

	c := dig.New()

	appConfig, err := config.LoadConfig()
	if err != nil {
		panic(err)
	}
	_ = c.Provide(func() *newrelic.Application {
		fmt.Println("Creating New Relic Application....")
		nrApp, _ := newrelic.NewApplication(
			newrelic.ConfigAppName(config.NewRelicAppName()),
			newrelic.ConfigLicense(config.NewRelicLicenseKey()),
			newrelic.ConfigDistributedTracerEnabled(true),
		)
		return nrApp
	})

	_ = c.Provide(func() *zap.Logger {
		return config.Logger
	})
	_ = c.Provide(func() *koanf.Koanf {
		return appConfig
	})
	_ = c.Provide(func() *http.Client {
		return &http.Client{}
	})
	//Provide Context
	_ = c.Provide(func() context.Context {
		return context.Background()
	})

	if err = c.Provide(config.NewWorkspaceServiceConfig); err != nil {
		config.Logger.Error("Error providing workspace service config", zap.Error(err))
		panic(err)
	}

	if err = c.Provide(config.NewAWSConfig); err != nil {
		config.Logger.Error("Error providing AWS config", zap.Error(err))
		panic(err)
	}

	if err = c.Provide(config.NewFileStoreConfig); err != nil {
		config.Logger.Error("Error providing FileStore config", zap.Error(err))
		panic(err)
	}

	if err = c.Provide(config.NewLocalFileStoreConfig); err != nil {
		config.Logger.Error("Error providing FileStore config", zap.Error(err))
		panic(err)
	}

	if err = c.Provide(config.NewS3FileStoreConfig); err != nil {
		config.Logger.Error("Error providing FileStore config", zap.Error(err))
		panic(err)
	}

	if err = c.Provide(config.NewAwsSession); err != nil {
		config.Logger.Error("Error providing FileStore config", zap.Error(err))
		panic(err)
	}

	if err = c.Provide(func(
		awsConfig *config.AWSConfig,
		storeConfig *config.FileStoreConfig,
		localFileStoreConfig *config.LocalFileStoreConfig,
		s3fileStoreConfig *config.S3FileStoreConfig,
		awsSession *session.Session,
		logger *zap.Logger,
	) filestore.FileStore {
		if storeConfig.GetFileStoreType() == constants.LOCAL {
			config.Logger.Info("Using local file store")
			lfs := impl.NewLocalFileStore(localFileStoreConfig, logger)
			return lfs
		} else {
			config.Logger.Info("Using s3 file store")
			s3fs := impl.NewS3FileSystem(awsSession, s3fileStoreConfig, logger)
			return s3fs
		}
	}); err != nil {
		config.Logger.Error("Error providing FileStore", zap.Error(err))
		panic(err)
	}

	err = c.Provide(config.NewJWTConfig)
	if err != nil {
		config.Logger.Error("Error providing JWT config", zap.Error(err))
		panic(err)
	}

	err = c.Provide(config.NewEnvConfig)
	if err != nil {
		config.Logger.Error("Error providing env config", zap.Error(err))
		panic(err)
	}

	err = c.Provide(workspace.NewWorkspaceServiceClient)
	if err != nil {
		config.Logger.Error("Error providing workspace service client", zap.Error(err))
		panic(err)
	}

	// Provide Asynq client
	err = c.Provide(func() *asynq.Client {
		return asynq.NewClient(asynq.RedisClientOpt{
			Addr: config.RedisAddress(),
			DB:   config.RedisDB(),
		})
	})
	if err != nil {
		panic(err)
	}

	// Provide GORM DB instance
	err = c.Provide(func() *gorm.DB {
		return config.InitDB()
	})
	if err != nil {
		panic(err)
	}
	//Provide Slack Alert For monitoring
	err = c.Provide(monitoring.NewSlackAlert)
	if err != nil {
		log.Println("Error providing slack alert:", err)
		return
	}

	// Provide GitnessClient
	err = c.Provide(func(logger *zap.Logger, slackAlert *monitoring.SlackAlert) *gitness_git_provider.GitnessClient {
		return gitness_git_provider.NewGitnessClient(config.GitnessURL(), config.GitnessToken(),
			client.NewHttpClient(), logger, slackAlert)
	})
	if err != nil {
		panic(err)
	}
	// Provide GitnessService
	err = c.Provide(func(client *gitness_git_provider.GitnessClient) *git_providers.GitnessService {
		return git_providers.NewGitnessService(client)
	})
	if err != nil {
		panic(err)
	}

	// Provide Repositories
	err = c.Provide(func(db *gorm.DB) (
		*repositories.ExecutionOutputRepository,
		*repositories.ProjectRepository,
		*repositories.ActivityLogRepository,
		*repositories.ExecutionRepository,
		*repositories.StoryRepository,
		*repositories.StoryFileRepository,
		*repositories.StoryInstructionRepository,
		*repositories.StoryTestCaseRepository,
		*repositories.ExecutionStepRepository,
		*repositories.OrganisationRepository,
		*repositories.PullRequestRepository,
		*repositories.PullRequestCommentsRepository,
		*repositories.LLMAPIKeyRepository,
		*repositories.DesignStoryReviewRepository,
	) {
		return repositories.NewExecutionOutputRepository(db),
			repositories.NewProjectRepository(db),
			repositories.NewActivityLogRepository(db),
			repositories.NewExecutionRepository(db),
			repositories.NewStoryRepository(db),
			repositories.NewStoryFileRepository(db),
			repositories.NewStoryInstructionRepository(db),
			repositories.NewStoryTestCaseRepository(db),
			repositories.NewExecutionStepRepository(db),
			repositories.NewOrganisationRepository(db),
			repositories.NewPullRequestRepository(db),
			repositories.NewPullRequestCommentsRepository(db),
			repositories.NewLLMAPIKeyRepository(db),
			repositories.NewDesignStoryReviewRepository(db)
	})
	if err != nil {
		panic(err)
	}

	// Provide Services
	err = c.Provide(func(activityLogRepo *repositories.ActivityLogRepository,
		executionRepo *repositories.ExecutionRepository) *services.ActivityLogService {
		return services.NewActivityLogService(activityLogRepo, executionRepo)
	})
	if err != nil {
		panic(err)
	}

	err = c.Provide(services.NewExecutionOutputService)
	if err != nil {
		panic(err)
	}

	err = c.Provide(services.NewExecutionService)
	if err != nil {
		panic(err)
	}

	err = c.Provide(services.NewLLMAPIKeyService)
	if err != nil {
		panic(err)
	}

	err = c.Provide(services.NewDesignStoryReviewService)
	if err != nil {
		panic(err)
	}
	err = c.Provide(func(organisationRepo *repositories.OrganisationRepository,
		gitnessService *git_providers.GitnessService) *services.OrganisationService {
		return services.NewOrganisationService(organisationRepo, gitnessService)
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("Starting to provide redis related repositories and services")

	// Provide Redis Client
	err = c.Provide(config.InitRedis)
	if err != nil {
		log.Println("Error providing Redis client:", err)
		panic(err)
	}

	fmt.Println("Now Redis related repositories....")

	// Provide Redis Repository
	err = c.Provide(repositories.NewProjectConnectionsRepository)
	if err != nil {
		log.Println("Error providing Redis repository:", err)
		panic(err)
	}

	// Provide ProjectService
	err = c.Provide(services.NewProjectService)
	if err != nil {
		fmt.Printf("Error providing ProjectService: %v\n", err)
		panic(err)
	}
	err = c.Provide(services.NewStoryService)
	if err != nil {
		fmt.Printf("Error providing StoryService: %v\n", err)
		panic(err)
	}
	err = c.Provide(services.NewPullRequestService)
	if err != nil {
		fmt.Printf("Error providing PullRequestService: %v\n", err)
		panic(err)
	}

	err = c.Provide(services.NewPullRequestCommentsService)
	if err != nil {
		fmt.Printf("Error providing PullRequestCommentsService: %v\n", err)
		panic(err)
	}

	//Provide ExecutionStepService
	err = c.Provide(services.NewExecutionStepService)

	// Provide Controllers
	err = c.Provide(controllers.NewAuthController)
	if err != nil {
		panic(err)
	}
	err = c.Provide(controllers.NewProjectController)
	err = c.Provide(controllers.NewStoryController)
	err = c.Provide(func() *controllers.HealthController {
		return controllers.NewHealth()
	})
	if err != nil {
		panic(err)
	}

	err = c.Provide(func(activityLogService *services.ActivityLogService) *controllers.ActivityLogController {
		return controllers.NewActivityLogController(activityLogService)
	})
	if err != nil {
		panic(err)
	}

	err = c.Provide(func(executionOutputService *services.ExecutionOutputService) *controllers.ExecutionOutputController {
		return controllers.NewExecutionOutputController(executionOutputService)
	})
	if err != nil {
		panic(err)
	}
	err = c.Provide(func(pullRequestCommentService *services.PullRequestCommentsService) *controllers.PullRequestCommentsController {
		return controllers.NewPullRequestCommentController(pullRequestCommentService)
	})
	err = c.Provide(func(executionService *services.ExecutionService) *controllers.ExecutionController {
		return controllers.NewExecutionController(executionService)
	})
	if err != nil {
		panic(err)
	}
	err = c.Provide(controllers.NewDesignStoryReviewController)
	err = c.Provide(controllers.NewPullRequestController)
	err = c.Provide(controllers.NewLLMAPIKeyController)

	if err = c.Provide(services.NewCodeDownloadService); err != nil {
		config.Logger.Error("Error providing CodeDownloadService", zap.Error(err))
		panic(err)
	}

	//Provide Middleware
	err = c.Provide(middleware.NewProjectAuthorizationMiddleware)
	if err != nil {
		config.Logger.Error("Error providing ProjectAuthorizationMiddleware", zap.Error(err))
		panic(err)
	}
	err = c.Provide(middleware.NewStoryAuthorizationMiddleware)
	if err != nil {
		config.Logger.Error("Error providing StoryAuthorizationMiddleware", zap.Error(err))
		panic(err)
	}
	err = c.Provide(middleware.NewOrganizationAuthorizationMiddleware)
	if err != nil {
		config.Logger.Error("Error providing OrganizationAuthorizationMiddleware", zap.Error(err))
		panic(err)
	}
	err = c.Provide(middleware.NewPullRequestAuthorizationMiddleware)
	if err != nil {
		config.Logger.Error("Error providing PullRequestAuthorizationMiddleware", zap.Error(err))
		panic(err)
	}

	err = c.Provide(services.NewProjectNotificationService)
	if err != nil {
		config.Logger.Error("Error providing ProjectNotificationService", zap.Error(err))
		panic(err)
	}

	//Websocket
	err = c.Provide(gateways.NewSocketIOServer)
	if err != nil {
		fmt.Printf("Error providing SocketIOServer: %v\n", err)
		panic(err)
	}
	fmt.Println("SocketIOServer provided")

	//Gateways
	err = c.Provide(gateways.NewWorkspaceGateway)
	if err != nil {
		fmt.Printf("Error providing WorkspaceGateway: %v\n", err)
		return
	}
	fmt.Println("WorkspaceGateway provided")

	// Provide Auth Middleware
	{
		err = c.Provide(config.NewGithubOAuthConfig)
		if err != nil {
			config.Logger.Error("Error providing github oauth config", zap.Error(err))
			panic(err)
		}

		err = c.Provide(auth.NewGithubAuthService)
		if err != nil {
			config.Logger.Error("Error providing github auth service", zap.Error(err))
			panic(err)
		}

		err = c.Provide(auth.NewEmailAuthService)
		if err != nil {
			config.Logger.Error("Error providing github auth service", zap.Error(err))
			panic(err)
		}

		err = c.Provide(auth.NewAuthenticator)
		if err != nil {
			config.Logger.Error("Error providing authenticator", zap.Error(err))
			panic(err)
		}

		if err = c.Provide(auth.NewAuthMiddleWare); err != nil {
			config.Logger.Error("Error providing AuthMiddleWare", zap.Error(err))
			panic(err)
		}
	}

	// User Controller
	{
		if err = c.Provide(repositories.NewUserRepository); err != nil {
			config.Logger.Error("Error providing UserRepository", zap.Error(err))
			panic(err)
		}

		if err = c.Provide(services.NewUserService); err != nil {
			config.Logger.Error("Error providing UserService", zap.Error(err))
			panic(err)
		}

		if err = c.Provide(controllers.NewUserController); err != nil {
			config.Logger.Error("Error providing UserController", zap.Error(err))
			panic(err)
		}

		if err = c.Provide(middleware.NewUserAuthorizationMiddleware); err != nil {
			config.Logger.Error("Error providing UserAuthorizationMiddleware", zap.Error(err))
			panic(err)
		}
	}

	// Setup routes and start the server
	err = c.Invoke(func(
		health *controllers.HealthController,
		auth *controllers.AuthController,
		projectsController *controllers.ProjectController,
		storiesController *controllers.StoryController,
		designStoryReviewCtrl *controllers.DesignStoryReviewController,
		llm_api_key *controllers.LLMAPIKeyController,
		asynqClient *asynq.Client,
		activityLogCtrl *controllers.ActivityLogController,
		executionOutputCtrl *controllers.ExecutionOutputController,
		executionCtrl *controllers.ExecutionController,
		pullRequestCtrl *controllers.PullRequestController,
		pullRequestCommentCtrl *controllers.PullRequestCommentsController,
		projectAuthMiddleware *middleware.ProjectAuthorizationMiddleware,
		storyAuthMiddleware *middleware.StoryAuthorizationMiddleware,
		orgAuthMiddleware *middleware.OrganizationAuthorizationMiddleware,
		pullRequestAuthMiddleware *middleware.PullRequestAuthorizationMiddleware,
		userService *services.UserService,
		organisationService *services.OrganisationService,
		ioServer *socketio.Server,
		nrApp *newrelic.Application,
		designStoryCtrl *controllers.DesignStoryReviewController,
		authenticator *auth.Authenticator,
		authMiddleware *auth.JWTAuthenticationMiddleware,
		userController *controllers.UserController,
		userAuthorizationMiddleware *middleware.UserAuthorizationMiddleware,
		logger *zap.Logger,
	) error {

		defer func() {
			err := asynqClient.Close()
			if err != nil {
				log.Println("Asynq Client closing error:", err)
			}
		}()

		env := config.Get("app.env")
		if env == constants.Development {
			err := InitializeSuperCoderData(userService, organisationService)
			if err != nil {
				log.Fatalf("Failed to initialize SuperCoder data: %v", err)
			}
		}

		r := gin.Default()

		r.Use(ginzap.Ginzap(logger, time.RFC3339, true))
		r.Use(ginzap.RecoveryWithZap(logger, true))

		// Add New Relic middleware to the Gin router
		r.Use(func(c *gin.Context) {
			txn := nrApp.StartTransaction(c.FullPath())
			defer txn.End()
			c.Set("newRelicTransaction", txn)
			c.Next()
		})

		r.Use(GinMiddleware("http://localhost:3000, https://developer.superagi.com"))
		r.RedirectTrailingSlash = false

		api := r.Group("/api")
		api.GET("/health", health.Health)

		githubAuth := api.Group("/github")
		githubAuth.GET("/signin", auth.GithubSignIn)
		githubAuth.GET("/callback", auth.GithubCallback)

		users := api.Group("/users", authMiddleware.MiddlewareFunc())
		users.GET("/details", userController.GetUserDetails)

		projects := api.Group("/projects", authMiddleware.MiddlewareFunc())

		projects.GET("", projectsController.GetAllProjects)
		projects.POST("", projectsController.CreateProject)
		projects.PUT("", projectsController.UpdateProject)

		projects.GET("/", projectsController.GetAllProjects)
		projects.POST("/", projectsController.CreateProject)
		projects.PUT("/", projectsController.UpdateProject)

		project := projects.Group("/:project_id", projectAuthMiddleware.Authorize())

		project.GET("", projectsController.GetProjectById)
		project.GET("/", projectsController.GetProjectById)

		project.GET("/download", projectsController.DownloadCode)
		project.GET("/pull-requests", pullRequestCtrl.GetAllPullRequestsByProjectID)
		project.GET("/stories", storiesController.GetAllStoriesOfProject)
		project.GET("/stories/in-progress", storiesController.GetInProgressStoriesByProjectId)
		project.GET("/design/stories", storiesController.GetDesignStoriesOfProject)

		stories := api.Group("/stories", authMiddleware.MiddlewareFunc())

		stories.POST("", storiesController.CreateStory)
		stories.POST("/", storiesController.CreateStory)
		stories.POST("/retrieve-code", storiesController.RetrieveCodeForFile)

		designStory := stories.Group("/design", authMiddleware.MiddlewareFunc())

		designStory.POST("", storiesController.CreateDesignStory)
		designStory.PUT("/edit", storiesController.EditDesignStoryById)
		designStory.PUT("/review_viewed/:story_id", storiesController.UpdateStoryIsReviewed)

		story := stories.Group("/:story_id", storyAuthMiddleware.Authorize())

		story.GET("", storiesController.GetStoryById)
		story.POST("", storiesController.EditStoryByID)
		story.DELETE("", storiesController.DeleteStoryById)

		story.GET("/", storiesController.GetStoryById)
		story.POST("/", storiesController.EditStoryByID)
		story.DELETE("/", storiesController.DeleteStoryById)

		story.GET("/code", storiesController.GetCodeForDesignStory)
		story.GET("/design", storiesController.GetDesignStoryByID)
		story.GET("/fetch-image", storiesController.GetImageByStoryId)

		story.GET("/execution-outputs", executionOutputCtrl.GetExecutionOutputsByStoryID)
		story.GET("/activity-logs", activityLogCtrl.GetActivityLogsByStoryID)
		story.PUT("/status", storiesController.UpdateStoryStatus)

		designReview := api.Group("/design/review", authMiddleware.MiddlewareFunc())
		designReview.POST("", designStoryReviewCtrl.CreateCommentForDesignStory)

		pullRequests := api.Group("/pull-requests", authMiddleware.MiddlewareFunc())

		pullRequests.POST("/create", pullRequestCtrl.CreateManualPullRequest)
		pullRequest := pullRequests.Group("/:pull_request_id", pullRequestAuthMiddleware.Authorize())
		pullRequest.GET("/diff", pullRequestCtrl.GetPullRequestDiffByPullRequestID)
		pullRequest.GET("/commits", pullRequestCtrl.FetchPullRequestCommits)
		pullRequest.POST("/comment", pullRequestCommentCtrl.CreateCommentForPrID)
		pullRequest.POST("/merge", pullRequestCtrl.MergePullRequest)

		llmApiKeys := api.Group("/llm_api_key", authMiddleware.MiddlewareFunc())
		llmApiKeys.POST("", llm_api_key.CreateLLMAPIKey)
		llmApiKeys.POST("/", llm_api_key.CreateLLMAPIKey)
		llmApiKeys.GET("", llm_api_key.FetchAllLLMAPIKeyByOrganisationID)
		llmApiKeys.GET("/", llm_api_key.FetchAllLLMAPIKeyByOrganisationID)

		authentication := api.Group("/auth")
		authentication.GET("/check_user", auth.CheckUser)
		authentication.POST("/sign_in", auth.SignIn)
		authentication.POST("/sign_up", auth.SignUp)
		authentication.POST("/logout", authMiddleware.LogoutHandler)

		// Wrap the socket.io server as Gin handlers for specific routes
		r.GET("/api/socket.io/*any", authMiddleware.MiddlewareFunc(), gin.WrapH(ioServer))
		r.POST("/api/socket.io/*any", authMiddleware.MiddlewareFunc(), gin.WrapH(ioServer))

		go func() {
			if err := ioServer.Serve(); err != nil {
				fmt.Printf("socket.io listen error: %s\n", err)
				log.Fatalf("socket.io listen error: %s\n", err)
			}
		}()
		defer func(ioServer *socketio.Server) {
			err := ioServer.Close()
			if err != nil {
				config.Logger.Error("Error closing socket.io server", zap.Error(err))
			}
		}(ioServer)

		fmt.Println("Starting Gin server on port 8080...")
		return r.Run()
	})

	if err != nil {
		fmt.Println("Error starting server:", err)
		panic(err)
	}
}

func GinMiddleware(allowOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "*")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Request.Header.Del("Origin")

		c.Next()
	}
}

func InitializeSuperCoderData(userService *services.UserService, organisationService *services.OrganisationService) error {
	organisation, err := organisationService.GetOrganisationByName("SuperCoderOrg")
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("error checking organisation: %v", err)
	}

	if organisation == nil {
		organisation, err = organisationService.CreateOrganisation(&models.Organisation{
			Name: "SuperCoderOrg",
		})
		if err != nil {
			return fmt.Errorf("error creating organisation: %v", err)
		}
	}

	user, err := userService.GetUserByEmail("supercoder@superagi.com")
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("error checking user: %v", err)
	}
	if user != nil {
		log.Println("User 'supercoder@superagi.com' already exists, skipping creation.")
		return nil
	}
	user = &models.User{
		Name:           "SuperCoderUser",
		Email:          "supercoder@superagi.com",
		OrganisationID: organisation.ID,
		Password:       "password",
	}
	user, err = userService.CreateUser(user)
	if err != nil {
		return fmt.Errorf("error creating user: %v", err)
	}
	return nil
}
