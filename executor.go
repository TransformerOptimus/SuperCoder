package main

import (
	"ai-developer/app/client"
	gitness_git_provider "ai-developer/app/client/git_provider"
	"ai-developer/app/client/workspace"
	"ai-developer/app/config"
	"ai-developer/app/llms"
	"ai-developer/app/monitoring"
	"ai-developer/app/repositories"
	"ai-developer/app/services"
	"ai-developer/app/services/git_providers"
	"ai-developer/app/workflow_executors"
	"ai-developer/app/workflow_executors/step_executors"
	"ai-developer/app/workflow_executors/step_executors/impl"
	"ai-developer/app/workflow_executors/step_executors/steps"
	"context"
	"github.com/hibiken/asynq"
	"github.com/knadh/koanf/v2"
	"go.uber.org/dig"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"log"
	"net/http"
	"os"
)

func main() {
	c := dig.New()
	config.InitLogger()
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
	//Provide Context
	_ = c.Provide(func() context.Context {
		return context.Background()
	})
	// Provide Asynq client
	err = c.Provide(func() *asynq.Client {
		return asynq.NewClient(asynq.RedisClientOpt{
			Addr: config.RedisAddress(),
			DB:   config.RedisDB(),
		})
	})
	if err != nil {
		log.Println("Error providing Asynq client:", err)
		panic(err)
	}
	err = c.Provide(config.NewWorkspaceServiceConfig)
	if err != nil {
		log.Println("Error providing workspace service config:", err)
		panic(err)
	}

	err = c.Provide(workspace.NewWorkspaceServiceClient)
	if err != nil {
		log.Println("Error providing workspace service:", err)
		panic(err)
	}

	// Provide GORM DB instance
	err = c.Provide(func() *gorm.DB {
		return config.InitDB()
	})
	if err != nil {
		panic(err)
	}

	// Config
	_ = c.Provide(config.NewAIDeveloperExecutionConfig)

	err = c.Provide(repositories.NewProjectRepository)
	if err != nil {
		log.Println("Error providing project repository:", err)
		panic(err)
	}
	err = c.Provide(repositories.NewOrganisationRepository)
	if err != nil {
		log.Println("Error providing organisation repository:", err)
		panic(err)
	}
	//Provide Repositories
	err = c.Provide(repositories.NewStoryRepository)
	if err != nil {
		log.Println("Error providing story repository:", err)
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
	err = c.Provide(repositories.NewExecutionRepository)
	if err != nil {
		log.Println("Error providing execution repository:", err)
		panic(err)
	}
	err = c.Provide(repositories.NewExecutionOutputRepository)
	if err != nil {
		log.Println("Error providing execution output repository:", err)
		panic(err)
	}
	err = c.Provide(repositories.NewPullRequestCommentsRepository)
	if err != nil {
		log.Println("Error providing pull request comments repository:", err)
		panic(err)
	}
	err = c.Provide(repositories.NewActivityLogRepository)
	if err != nil {
		log.Println("Error providing activity log repository:", err)
		panic(err)
	}
	err = c.Provide(repositories.NewStoryInstructionRepository)
	if err != nil {
		log.Println("Error providing story instruction repository:", err)
		panic(err)
	}
	err = c.Provide(repositories.NewPullRequestRepository)
	if err != nil {
		log.Println("Error providing pull request repository:", err)
		panic(err)
	}
	err = c.Provide(repositories.NewExecutionStepRepository)
	if err != nil {
		log.Println("Error providing execution step repository:", err)
		panic(err)
	}
	err = c.Provide(repositories.NewLLMAPIKeyRepository)
	if err != nil {
		log.Println("Error providing llm api key repository:", err)
		panic(err)
	}
	// Provide Redis Client
	err = c.Provide(config.InitRedis)
	if err != nil {
		log.Println("Error providing Redis client:", err)
		panic(err)
	}

	// Provide Redis Repository
	err = c.Provide(repositories.NewProjectConnectionsRepository)
	if err != nil {
		log.Println("Error providing Redis repository:", err)
		panic(err)
	}
	// Provide GitnessClient
	err = c.Provide(func(logger *zap.Logger, slackAlert *monitoring.SlackAlert) *gitness_git_provider.GitnessClient {
		return gitness_git_provider.NewGitnessClient(config.GitnessURL(), config.GitnessToken(), client.NewHttpClient(), logger, slackAlert)
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

	//Provide Services
	_ = c.Provide(services.NewOrganisationService)
	_ = c.Provide(services.NewProjectService)
	_ = c.Provide(services.NewExecutionService)
	_ = c.Provide(services.NewExecutionOutputService)
	_ = c.Provide(services.NewPullRequestService)
	_ = c.Provide(services.NewActivityLogService)
	_ = c.Provide(services.NewStoryService)
	_ = c.Provide(services.NewPullRequestCommentsService)
	_ = c.Provide(services.NewExecutionStepService)
	_ = c.Provide(services.NewPullRequestService)
	_ = c.Provide(services.NewExecutionOutputService)
	_ = c.Provide(services.NewLLMAPIKeyService)

	//GenerateCodeStep
	err = c.Provide(impl.NewOpenAIFlaskCodeGenerator)
	if err != nil {
		log.Println("Error providing generate code step:", err)
		panic(err)
	}
	//UpdateCodeFilesStep
	err = c.Provide(impl.NewUpdateCodeFileExecutor)
	if err != nil {
		log.Println("Error providing update code file step:", err)
		panic(err)
	}
	//GitMakeBranchStep
	err = c.Provide(impl.NewGitMakeBranchExecutor)
	if err != nil {
		log.Println("Error providing git make branch step:", err)
		panic(err)

	}
	//serverStartTestStep
	err = c.Provide(impl.NewFlaskServerStartTestExecutor)
	if err != nil {
		log.Println("Error providing server start test step:", err)
		panic(err)

	}
	//GitCommitStep
	err = c.Provide(impl.NewGitCommitExecutor)
	if err != nil {
		log.Println("Error providing git commit step:", err)
		panic(err)
	}
	//GitPushStep
	err = c.Provide(impl.NewGitPushExecutor)
	if err != nil {
		log.Println("Error providing git push step:", err)
		panic(err)
	}
	//GitMakePullRequestStep
	err = c.Provide(impl.NewGitnessMakePullRequestExecutor)
	if err != nil {
		log.Println("Error providing git make pull request step:", err)
		panic(err)
	}
	err = c.Provide(impl.NewResetFlaskDBStepExecutor)
	if err != nil {
		log.Println("Error providing reset flask db step:", err)
		panic(err)
	}

	//Provide Slack Alert For monitoring
	err = c.Provide(monitoring.NewSlackAlert)
	if err != nil {
		log.Println("Error providing slack alert:", err)
		return
	}

	// Provide OpenAiClient
	if err := c.Provide(func() *llms.OpenAiClient {
		apiKey := config.OpenAIAPIKey()
		log.Println("OpenAI API Key:", apiKey)
		return llms.NewOpenAiClient(apiKey)
	}); err != nil {
		log.Fatalf("Error providing OpenAiClient: %v", err)
	}

	if template, exists := os.LookupEnv("EXECUTION_TEMPLATE"); template == "FLASK" || !exists {
		_ = c.Provide(func(
			openAIFlaskCodeGenerator *impl.OpenAIFlaskCodeGenerator,
			gitMakeBranchExecutor *impl.GitMakeBranchExecutor,
			updateCodeFileExecutor *impl.UpdateCodeFileExecutor,
			flaskServerStartTestExecutor *impl.FlaskServerStartTestExecutor,
			gitCommitExecutor *impl.GitCommitExecutor,
			gitPushExecutor *impl.GitPushExecutor,
			gitnessMakePullRequestExecutor *impl.GitnessMakePullRequestExecutor,
			resetFlaskDBStepExecutor *impl.ResetFlaskDBStepExecutor,
		) map[steps.StepName]step_executors.StepExecutor {
			return map[steps.StepName]step_executors.StepExecutor{
				steps.CODE_GENERATE_STEP:           *openAIFlaskCodeGenerator,
				steps.UPDATE_CODE_FILE_STEP:        *updateCodeFileExecutor,
				steps.GIT_COMMIT_STEP:              *gitCommitExecutor,
				steps.GIT_CREATE_BRANCH_STEP:       *gitMakeBranchExecutor,
				steps.GIT_PUSH_STEP:                *gitPushExecutor,
				steps.GIT_CREATE_PULL_REQUEST_STEP: *gitnessMakePullRequestExecutor,
				steps.SERVER_START_STEP:            *flaskServerStartTestExecutor,
				steps.RETRY_CODE_GENERATE_STEP:     *openAIFlaskCodeGenerator,
				steps.RESET_DB_STEP:                *resetFlaskDBStepExecutor,
			}
		})
	}

	_ = c.Provide(workflow_executors.NewWorkflowExecutor)

	err = c.Invoke(func(
		adec *config.AIDeveloperExecutionConfig,
		db *gorm.DB,
		alert *monitoring.SlackAlert,
		executor *workflow_executors.WorkflowExecutor,
	) error {
		if _, err := config.LoadConfig(); err != nil {
			return err
		}
		log.Println("Going to execute AI Developer Workflow Execution For Flask")
		err = executor.Execute(
			workflow_executors.FlaskWorkflowConfig,
			&workflow_executors.WorkflowExecutionArgs{
				StoryId:       adec.GetStoryID(),
				IsReExecution: adec.IsReExecution(),
				Branch:        adec.GetBranch(),
				PullRequestId: adec.GetPullRequestID(),
				ExecutionId:   adec.GetExecutionID(),
			},
		)
		return err
	})

	if err != nil {
		log.Fatalf("could not run server: %v", err)
	}
}
