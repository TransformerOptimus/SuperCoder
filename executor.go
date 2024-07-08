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
	"ai-developer/app/services/s3_providers"
	"ai-developer/app/workflow_executors"
	"ai-developer/app/workflow_executors/step_executors"
	"ai-developer/app/workflow_executors/step_executors/impl"
	"ai-developer/app/workflow_executors/step_executors/steps"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/hibiken/asynq"
	"github.com/knadh/koanf/v2"
	"go.uber.org/dig"
	"go.uber.org/zap"
	"gorm.io/gorm"
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
	err = c.Provide(repositories.NewDesignStoryReviewRepository)
	if err != nil {
		log.Println("Error providing design story review repository:", err)
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
	_ = c.Provide(s3_providers.NewS3Service)
	_ = c.Provide(services.NewDesignStoryReviewService)
	fmt.Println("Services Successfully Provided.")

	//GenerateCodeStep
	err = c.Provide(impl.NewOpenAICodeGenerator)
	if err != nil {
		log.Println("Error providing generate code step:", err)
		panic(err)
	}
	err = c.Provide(impl.NewOpenAINextJsCodeGenerationExecutor)
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
	err = c.Provide(impl.NewNextJsUpdateCodeFileExecutor)
	if err != nil {
		log.Println("Error providing next js code file step:", err)
		panic(err)
	}
	//GitMakeBranchStep
	err = c.Provide(impl.NewGitMakeBranchExecutor)
	if err != nil {
		log.Println("Error providing git make branch step:", err)
		panic(err)

	}
	//FLASK serverStartTestStep
	err = c.Provide(impl.NewFlaskServerStartTestExecutor)
	if err != nil {
		log.Println("Error providing server start test step:", err)
		panic(err)

	}
	//DJANGO serverStartTestStep
	err = c.Provide(impl.NewDjangoServerStartTestExecutor)
	if err != nil {
		log.Println("Error providing server start test step:", err)
		panic(err)

	}
	//NEXT JS serverStartTestStep
	err = c.Provide(impl.NewNextJsServerStartTestExecutor)
	if err != nil {
		log.Println("Error providing next js test step:", err)
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
	err = c.Provide(impl.NewPackageInstallStepExecutor)
	if err != nil {
		log.Println("Error providing package install step:", err)
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
		return llms.NewOpenAiClient(apiKey)
	}); err != nil {
		log.Fatalf("Error providing OpenAiClient: %v", err)
	}

	// Provide ClaudeClient
	if err = c.Provide(func() *llms.ClaudeClient {
		apiKey := config.ClaudeAPIKey()
		return llms.NewClaudeClient(apiKey)
	}); err != nil {
		log.Fatalf("Error providing ClaudeClient: %v", err)
	}
	template, exists := os.LookupEnv("EXECUTION_TEMPLATE")
	if template == "FLASK" || !exists {
		_ = c.Provide(func(
			openAICodeGenerator *impl.OpenAICodeGenerator,
			gitMakeBranchExecutor *impl.GitMakeBranchExecutor,
			updateCodeFileExecutor *impl.UpdateCodeFileExecutor,
			flaskServerStartTestExecutor *impl.FlaskServerStartTestExecutor,
			gitCommitExecutor *impl.GitCommitExecutor,
			gitPushExecutor *impl.GitPushExecutor,
			gitnessMakePullRequestExecutor *impl.GitnessMakePullRequestExecutor,
			resetFlaskDBStepExecutor *impl.ResetFlaskDBStepExecutor,
			poetryPackageInstallStepExecutor *impl.PackageInstallStepExecutor,
		) map[steps.StepName]step_executors.StepExecutor {
			return map[steps.StepName]step_executors.StepExecutor{
				steps.CODE_GENERATE_STEP:           *openAICodeGenerator,
				steps.UPDATE_CODE_FILE_STEP:        *updateCodeFileExecutor,
				steps.GIT_COMMIT_STEP:              *gitCommitExecutor,
				steps.GIT_CREATE_BRANCH_STEP:       *gitMakeBranchExecutor,
				steps.GIT_PUSH_STEP:                *gitPushExecutor,
				steps.GIT_CREATE_PULL_REQUEST_STEP: *gitnessMakePullRequestExecutor,
				steps.SERVER_START_STEP:            *flaskServerStartTestExecutor,
				steps.RETRY_CODE_GENERATE_STEP:     *openAICodeGenerator,
				steps.RESET_DB_STEP:                *resetFlaskDBStepExecutor,
				steps.PACKAGE_INSTALL_STEP:         *poetryPackageInstallStepExecutor,
			}
		})
	} else if template == "DJANGO" {
		_ = c.Provide(func(
			openAICodeGenerator *impl.OpenAICodeGenerator,
			gitMakeBranchExecutor *impl.GitMakeBranchExecutor,
			updateCodeFileExecutor *impl.UpdateCodeFileExecutor,
			djangoServerStartTestExecutor *impl.DjangoServerStartTestExecutor,
			gitCommitExecutor *impl.GitCommitExecutor,
			gitPushExecutor *impl.GitPushExecutor,
			gitnessMakePullRequestExecutor *impl.GitnessMakePullRequestExecutor,
		) map[steps.StepName]step_executors.StepExecutor {
			return map[steps.StepName]step_executors.StepExecutor{
				steps.CODE_GENERATE_STEP:           *openAICodeGenerator,
				steps.UPDATE_CODE_FILE_STEP:        *updateCodeFileExecutor,
				steps.GIT_COMMIT_STEP:              *gitCommitExecutor,
				steps.GIT_CREATE_BRANCH_STEP:       *gitMakeBranchExecutor,
				steps.GIT_PUSH_STEP:                *gitPushExecutor,
				steps.GIT_CREATE_PULL_REQUEST_STEP: *gitnessMakePullRequestExecutor,
				steps.SERVER_START_STEP:            *djangoServerStartTestExecutor,
				steps.RETRY_CODE_GENERATE_STEP:     *openAICodeGenerator,
			}
		})
	} else if template == "NEXTJS" {
		_ = c.Provide(func(
			openAiNextJsCodeGenerator *impl.OpenAiNextJsCodeGenerator,
			updateCodeFileExecutor *impl.NextJsUpdateCodeFileExecutor,
			serverStartExecutor *impl.NextJsServerStartTestExecutor,
		) map[steps.StepName]step_executors.StepExecutor {
			return map[steps.StepName]step_executors.StepExecutor{
				steps.CODE_GENERATE_CSS_STEP:       *openAiNextJsCodeGenerator,
				steps.UPDATE_CODE_CSS_FILE_STEP:    *updateCodeFileExecutor,
				steps.CODE_GENERATE_LAYOUT_STEP:    *openAiNextJsCodeGenerator,
				steps.UPDATE_CODE_LAYOUT_FILE_STEP: *updateCodeFileExecutor,
				steps.CODE_GENERATE_PAGE_STEP:      *openAiNextJsCodeGenerator,
				steps.UPDATE_CODE_PAGE_FILE_STEP:   *updateCodeFileExecutor,
				steps.SERVER_START_STEP:            *serverStartExecutor,
				steps.RETRY_CODE_GENERATE_STEP:     *openAiNextJsCodeGenerator,
				steps.UPDATE_CODE_FILE_STEP:        *updateCodeFileExecutor,
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
		log.Println(fmt.Sprintf("Going to execute AI Developer Workflow Execution For %s", template))
		if template == "FLASK" {
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
		} else if template == "DJANGO" {
			err = executor.Execute(
				workflow_executors.DjangoWorkflowConfig,
				&workflow_executors.WorkflowExecutionArgs{
					StoryId:       adec.GetStoryID(),
					IsReExecution: adec.IsReExecution(),
					Branch:        adec.GetBranch(),
					PullRequestId: adec.GetPullRequestID(),
					ExecutionId:   adec.GetExecutionID(),
				},
			)
			return err
		} else if template == "NEXTJS" {
			log.Println("Going to execute AI Developer Next JS Workflow Execution")
			err = executor.Execute(
				workflow_executors.NextJsWorkflowConfig,
				&workflow_executors.WorkflowExecutionArgs{
					StoryId:       adec.GetStoryID(),
					IsReExecution: adec.IsReExecution(),
					ExecutionId:   adec.GetExecutionID(),
				})
		} else {
			fmt.Println("_____Invalid template_____", template)
		}
		
		return nil
	})

	if err != nil {
		log.Fatalf("could not run server: %v", err)
	}
}
