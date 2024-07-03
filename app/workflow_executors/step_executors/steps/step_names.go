package steps

type StepName string

var (
	CODE_GENERATE_STEP           StepName = "CODE_GENERATE_STEP"
	RETRY_CODE_GENERATE_STEP     StepName = "RETRY_CODE_GENERATE_STEP"
	GIT_COMMIT_STEP              StepName = "GIT_COMMIT_STEP"
	GIT_CREATE_BRANCH_STEP       StepName = "GIT_CREATE_BRANCH_STEP"
	GIT_PUSH_STEP                StepName = "GIT_PUSH_STEP"
	GIT_CREATE_PULL_REQUEST_STEP StepName = "GIT_CREATE_PULL_REQUEST_STEP"
	SERVER_START_STEP            StepName = "SERVER_START_STEP"
	UPDATE_CODE_FILE_STEP        StepName = "UPDATE_CODE_FILE_STEP"
	RESET_DB_STEP                StepName = "RESET_DB_STEP"
)

func (s StepName) String() string {
	return string(s)
}
