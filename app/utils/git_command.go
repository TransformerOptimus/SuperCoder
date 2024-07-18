package utils

import (
	"ai-developer/app/config"
	"ai-developer/app/constants"
	"ai-developer/app/models"
	"fmt"
	"os/exec"
	"strings"
)

func ConfigureGitUserName(workingDir string) error {
	fmt.Printf("Setting Git global user.name\n")
	cmd := exec.Command("git", "config", "--global", "user.name", "SuperCoder")
	cmd.Dir = workingDir
	if _, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set global user.name: %s", err)
	}
	return nil
}

func ConfigGitUserEmail(workingDir string) error {
	fmt.Printf("Setting Git global user.email\n")
	cmd := exec.Command("git", "config", "--global", "user.email", "supercoder@superagi.com")
	cmd.Dir = workingDir
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("Error setting global user.email: %s\n", string(output))
		return fmt.Errorf("failed to set global user.email: %s", err)
	}
	return nil
}

func ConfigGitSafeDir(workingDir string) error {
	cmd := exec.Command("git", "config", "--global", "--add", "safe.directory", workingDir)
	cmd.Dir = workingDir
	if _, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set safe.directory config: %s", err)
	}
	return nil
}

func ConfigGitPullRebaseTrue(workingDir string) error {
	cmd := exec.Command("git", "config", "--global", "pull.rebase", "true")
	cmd.Dir = workingDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error configuring Git pull rebase false: %s, output: %s", err.Error(), string(output))
	}
	return nil
}

func InitialiseGit(workingDir string) (*exec.Cmd, error) {
	fmt.Printf("Initializing Git repository in the working directory: %s\n", workingDir)
	cmd := exec.Command("git", "init")
	cmd.Dir = workingDir
	if log, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("Log : ", log)
		fmt.Printf("Error initializing Git repository: %s\n", err)
		return nil, err
	}

	return cmd, nil
}

func CreateBranch(workingDir string, branchName string) error {
	fmt.Printf("Creating new branch '%s'\n", branchName)
	cmd := exec.Command("git", "checkout", "-b", branchName)
	cmd.Dir = workingDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), fmt.Sprintf("fatal: a branch named '%s' already exists", branchName)) {
			fmt.Printf("Branch '%s' already exists. Checking out existing branch.\n", branchName)
			return CheckoutBranch(workingDir, branchName)
		}
		return fmt.Errorf("git checkout -b error: %s, output: %s", err.Error(), string(output))
	}
	return nil
}

func CheckoutBranch(workingDir string, branchName string) error {
	fmt.Printf("Checking out branch '%s'\n", branchName)
	cmd := exec.Command("git", "checkout", branchName)
	cmd.Dir = workingDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), fmt.Sprintf("error: pathspec '%s' did not match any file(s) known to git", branchName)) {
			//fmt.Errorf("branch '%s' does not exist", branchName)
			fmt.Printf("Creating new branch '%s'\n", branchName)
			return CreateBranch(workingDir, branchName)
		}
		return fmt.Errorf("git checkout error: %s, output: %s", err.Error(), string(output))
	}
	return nil
}

func PullBranch(workingDir string, origin string, branchName string) error {
	fmt.Printf("Executing git pull origin %s %s\n", origin, branchName)
	cmd := exec.Command("git", "pull", origin, branchName, "--allow-unrelated-histories")
	cmd.Dir = workingDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull error: %s, output: %s", err.Error(), string(output))
	}
	fmt.Printf("Output: %s\n", string(output))
	return nil
}

func PullOriginMain(workingDir string, origin string) error {
	fmt.Printf("Executing git pull origin main --no-rebase",)
	cmd := exec.Command("git", "pull", origin, "main", "--no-rebase")
	cmd.Dir = workingDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull error: %s, output: %s", err.Error(), string(output))
	}
	fmt.Printf("Output: %s\n", string(output))
	return nil
}

func PullOriginBranch(workingDir string, project *models.Project, GitnessSpaceOrProjectName string) error {
	branchName := "main"
	httpPrefix := "https"

	if config.AppEnv() == constants.Development {
		httpPrefix = "http"
	}
	origin := fmt.Sprintf("%s://%s:%s@%s/git/%s/%s.git", httpPrefix, config.GitnessUser(), config.GitnessToken(), config.GitnessHost(), GitnessSpaceOrProjectName, project.Name)
	fmt.Printf("User: %s, Token: %s, Host: %s, Space/Project: %s, Project: %s\n", config.GitnessUser(), config.GitnessToken(), config.GitnessHost(), GitnessSpaceOrProjectName, project.Name)
	fmt.Printf("Origin: %s\n", origin)
	err := PullBranch(workingDir, origin, branchName)
	if err != nil {
		return fmt.Errorf("error pulling latest main: %s", err.Error())
	}
	return nil
}

func GetCurrentBranch(workingDir string) (string, error) {
    fmt.Printf("Getting current branch in directory: %s\n", workingDir)
    cmd := exec.Command("git", "branch", "--show-current")
    cmd.Dir = workingDir
    output, err := cmd.CombinedOutput()
    if err != nil {
        exitError, ok := err.(*exec.ExitError)
        if ok {
            return "", fmt.Errorf("git command failed with exit code %d: %s", exitError.ExitCode(), string(output))
        }
        return "", fmt.Errorf("git command failed: %v\nOutput: %s", err, string(output))
    }
    return strings.TrimSpace(string(output)), nil
}

func GitAddToTrackFiles(workingDir string, err error) (string, error) {
	fmt.Printf("Making a commit in directory: %s\n", workingDir)
	cmd := exec.Command("git", "add", ".")
	fmt.Println("Git add command: ", cmd.String())
	cmd.Dir = workingDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error adding changes: %s\n", err)
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func GitCommitWithMessage(workingDir string, commitMessage string, err error) (string, error) {
	cmd := exec.Command("git", "commit", "-m", commitMessage)
	fmt.Println("Git commit command: ", cmd.String())
	cmd.Dir = workingDir
	output, err := cmd.CombinedOutput()
	outputStr := strings.TrimSpace(string(output))
	if err != nil {
		if strings.Contains(outputStr, "nothing to commit, working tree clean") {
			return "Nothing to commit, working tree clean", nil
		}
		fmt.Printf("Error committing changes: %s\n", outputStr)
		fmt.Println("Error committing changes: ", err)
		return "", err
	}
	return outputStr, nil
}

func GetLatestCommitID(workingDir string, err error) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	fmt.Println("Git rev-parse command: ", cmd.String())
	cmd.Dir = workingDir
	commitIDOutput, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(commitIDOutput)), nil
}

func GitPush(workingDir, origin, branch string) error {
	fmt.Printf("Pushing changes to remote repository: %s/%s\n", origin, branch)
	cmd := exec.Command("git", "push", origin, branch)
	cmd.Dir = workingDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err.Error(), string(output))
	}
	return nil
}
