package main

import (
	"ai-developer/app/client"
	gitness_git_provider "ai-developer/app/client/git_provider"
	"ai-developer/app/client/workspace"
	"ai-developer/app/config"
	"ai-developer/app/constants"
	"ai-developer/app/monitoring"
	"ai-developer/app/repositories"
	"ai-developer/app/services"
	"ai-developer/app/services/git_providers"
	"ai-developer/app/services/s3_providers"
	"ai-developer/app/tasks"
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/hibiken/asynq"
	"github.com/knadh/koanf/v2"
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

	_ = c.Provide(func() *zap.Logger {
		return config.Logger
	})

	_ = c.Provide(func() *koanf.Koanf {
		return appConfig
	})

	_ = c.Provide(func() *http.Client {
		return &http.Client{}
	})

	_ = c.Provide(client.NewHttpClient)

	//Provide Context
	_ = c.Provide(func() context.Context {
		return context.Background()
	})

	// Provide Redis Client
	err = c.Provide(config.InitRedis)
	if err != nil {
		log.Println("Error providing Redis client:", err)
		panic(err)
	}

	fmt.Println("Now Redis related repositories....")

	// Provide GORM DB instance
	err = c.Provide(func() *gorm.DB {
		return config.InitDB()
	})
	if err != nil {
		panic(err)
	}

	//Provide Repositories
	err = c.Provide(repositories.NewExecutionRepository)
	if err != nil {
		log.Println("Error providing execution repository:", err)
		panic(err)
	}
	//executionOutputRepo *repositories.ExecutionOutputRepository,
	err = c.Provide(repositories.NewExecutionOutputRepository)
	if err != nil {
		log.Println("Error providing execution output repository:", err)
		panic(err)
	}
	//pullRequestRepo *repositories.PullRequestRepository,
	err = c.Provide(repositories.NewPullRequestRepository)
	if err != nil {
		log.Println("Error providing pull request repository:", err)
		panic(err)
	}
	//ActivityLogRepository
	err = c.Provide(repositories.NewActivityLogRepository)
	if err != nil {
		log.Println("Error providing activity log repository:", err)
		panic(err)
	}
	//ProjectRepository
	err = c.Provide(repositories.NewProjectRepository)
	if err != nil {
		log.Println("Error providing project repository:", err)
		panic(err)
	}
	//OrganisationRepository
	err = c.Provide(repositories.NewOrganisationRepository)
	if err != nil {
		log.Println("Error providing organisation repository:", err)
		panic(err)
	}
	//StoryRepository
	err = c.Provide(repositories.NewStoryRepository)
	if err != nil {
		log.Println("Error providing story repository:", err)
		panic(err)
	}
	//Story Instructions Repository
	err = c.Provide(repositories.NewStoryInstructionRepository)
	if err != nil {
		log.Println("Error providing story instruction repository:", err)
		panic(err)
	}
	//StoryTestCaseRepository
	err = c.Provide(repositories.NewStoryTestCaseRepository)
	if err != nil {
		log.Println("Error providing story test case repository:", err)
		panic(err)
	}
	//StoryFileRepository
	err = c.Provide(repositories.NewStoryFileRepository)
	if err != nil {
		log.Println("Error providing story file repository:", err)
		panic(err)
	}
	//ExecutionStepRepository
	err = c.Provide(repositories.NewExecutionStepRepository)
	if err != nil {
		log.Println("Error providing execution step repository:", err)
		panic(err)
	}
	//NewProjectConnectionsRepository
	err = c.Provide(repositories.NewProjectConnectionsRepository)
	if err != nil {
		log.Println("Error providing project connections repository:", err)
		panic(err)
	}
	//Pull Request Repository
	err = c.Provide(repositories.NewPullRequestCommentsRepository)
	if err != nil {
		log.Println("Error providing pull request comments repository:", err)
		panic(err)
	}
	//NewLLMAPIKeyRepository
	err = c.Provide(repositories.NewLLMAPIKeyRepository)
	if err != nil {
		log.Println("Error providing LLM API Key repository:", err)
		panic(err)
	}

	fmt.Println("Worker - Providing workspace service client...")
	err = c.Provide(config.NewWorkspaceServiceConfig)
	if err != nil {
		log.Println("Error providing workspace service config:", err)
		panic(err)
	}
	fmt.Printf("Worker - Providing workspace service client...")
	err = c.Provide(workspace.NewWorkspaceServiceClient)
	if err != nil {
		log.Println("Error providing workspace service:", err)
		panic(err)
	}
	err = c.Provide(services.NewActivityLogService)
	if err != nil {
		log.Println("Error providing activity log service:", err)
		panic(err)
	}
	err = c.Provide(services.NewStoryService)
	if err != nil {
		log.Println("Error providing story service:", err)
		panic(err)
	}
	err = c.Provide(services.NewExecutionService)
	if err != nil {
		log.Println("Error providing execution service:", err)
		panic(err)
	}
	err = c.Provide(services.NewProjectService)
	if err != nil {
		log.Println("Error providing project service:", err)
		panic(err)
	}
	err = c.Provide(services.NewExecutionStepService)
	if err != nil {
		log.Println("Error providing execution step service:", err)
		panic(err)
	}
	err = c.Provide(services.NewExecutionOutputService)
	if err != nil {
		fmt.Println("Error providing ExecutionOutputService: ", err)
		panic(err)
	}
	err = c.Provide(services.NewPullRequestService)
	if err != nil {
		fmt.Printf("Error providing PullRequestService: %v\n", err)
		panic(err)
	}
	//NewLLMAPIKeyService
	err = c.Provide(services.NewLLMAPIKeyService)
	if err != nil {
		fmt.Println("Error providing LLM API Key Service: ", err)
		panic(err)
	}
	// Provide GitnessClient
	err = c.Provide(func(logger *zap.Logger, slackAlert *monitoring.SlackAlert) *gitness_git_provider.GitnessClient {
		return gitness_git_provider.NewGitnessClient(config.GitnessURL(), config.GitnessToken(), client.NewHttpClient(), logger, slackAlert)
	})
	if err != nil {
		panic(err)
	}
	err = c.Provide(s3_providers.NewS3Service)
	if err != nil {
		fmt.Println("Error providing S3 service:", err)
		panic(err)
	}
	// Provide GitnessService
	err = c.Provide(func(client *gitness_git_provider.GitnessClient) *git_providers.GitnessService {
		return git_providers.NewGitnessService(client)
	})

	// Provide Asynq client
	err = c.Provide(func() *asynq.Client {
		return asynq.NewClient(asynq.RedisClientOpt{
			Addr: config.RedisAddress(),
			DB:   config.RedisDB(),
		})
	})
	if err != nil {
		log.Fatalf("could not provide *asynq.Client: %v", err)
	}

	// Provide Slack Alert For monitoring
	err = c.Provide(monitoring.NewSlackAlert)
	if err != nil {
		log.Println("Error providing slack alert:", err)
		return
	}
	//Provide DeleteWorkspaceTaskHandler
	err = c.Provide(tasks.NewDeleteWorkspaceTaskHandler)
	if err != nil {
		log.Fatalf("could not provide DeleteWorkspaceTaskHandler: %v", err)
	}

	//Provide CreateExecutionJobTaskHandler
	err = c.Provide(tasks.NewCreateExecutionJobTaskHandler)
	if err != nil {
		log.Fatalf("could not provide CreateExecutionJobTaskHandler: %v", err)
	}

	err = c.Provide(tasks.NewCheckExecutionStatusTaskHandler)
	if err != nil {
		log.Fatalf("could not provide CheckExecutionStatusTaskHandler: %v", err)
	}
	//Provide asynq scheduler
	err = c.Provide(func() *asynq.Scheduler {
		return asynq.NewScheduler(asynq.RedisClientOpt{
			Addr: config.RedisAddress(),
			DB:   config.RedisDB(),
		}, nil)
	})

	err = c.Provide(func(
		deleteWorkspaceTaskHandler *tasks.DeleteWorkspaceTaskHandler,
		createExecutionJobTaskHandler *tasks.CreateExecutionJobTaskHandler,
		checkExecutionStatusTaskHandler *tasks.CheckExecutionStatusTaskHandler,
		workspaceServiceClient *workspace.WorkspaceServiceClient,
		projectService *services.ProjectService,
		logger *zap.Logger,
	) *asynq.ServeMux {
		mux := asynq.NewServeMux()
		mux.HandleFunc(constants.DeleteWorkspaceTaskType, deleteWorkspaceTaskHandler.HandleTask)
		mux.HandleFunc(constants.CreateExecutionJobTaskType, createExecutionJobTaskHandler.HandleTask)
		mux.HandleFunc(constants.CheckExecutionStatusTaskType, checkExecutionStatusTaskHandler.HandleTask)
		return mux
	})

	if err != nil {
		log.Fatalf("could not provide *asynq.ServeMux: %v", err)
	}
	err = c.Provide(func() *asynq.Server {
		return asynq.NewServer(
			asynq.RedisClientOpt{
				Addr: config.RedisAddress(),
				DB:   config.RedisDB(),
			},
			asynq.Config{
				Concurrency: 10,
				Queues: map[string]int{
					"critical": 6,
					"default":  3,
					"low":      1,
				},
				StrictPriority: true,
			},
		)
	})

	err = c.Invoke(func(srv *asynq.Server, mux *asynq.ServeMux, scheduler *asynq.Scheduler,
	) error {
		task := asynq.NewTask(constants.CheckExecutionStatusTaskType, nil, asynq.TaskID(constants.CheckExecutionStatusTaskType))
		if _, err := scheduler.Register("*/30 * * * *", task); err != nil {
			log.Fatalf("could not schedule task: %v", err)
		}

		// Start the scheduler in a separate goroutine
		go func() {
			if err := scheduler.Run(); err != nil {
				log.Fatalf("could not run scheduler: %v", err)
			}
		}()

		// Start the server
		return srv.Run(mux)
	})
	if err != nil {
		log.Fatalf("could not run server: %v", err)
	}
}
