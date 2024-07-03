package impl

import (
	"ai-developer/app/config"
	"ai-developer/app/services"
	"ai-developer/app/workflow_executors/step_executors/steps"
	"bufio"
	"bytes"
	"fmt"
	"go.uber.org/zap"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type FlaskServerStartTestExecutor struct {
	executionStepService *services.ExecutionStepService
	activityLogService   *services.ActivityLogService
	logger               *zap.Logger
}

func NewFlaskServerStartTestExecutor(
	executionStepService *services.ExecutionStepService,
	activityLogService *services.ActivityLogService,
	logger *zap.Logger,
) *FlaskServerStartTestExecutor {
	return &FlaskServerStartTestExecutor{
		executionStepService: executionStepService,
		activityLogService:   activityLogService,
		logger:               logger,
	}
}

func (e FlaskServerStartTestExecutor) Execute(step steps.ServerStartTestStep) error {
	fmt.Printf("Executing Server Start Test Step: %s\n", step.StepName())

	err := e.activityLogService.CreateActivityLog(
		step.ExecutionStep.ExecutionID,
		step.ExecutionStep.ID,
		"INFO",
		fmt.Sprintf("Starting and testing Server..."),
	)
	if err != nil {
		fmt.Println("Error creating activity log" + err.Error())
		return err
	}
	status, stderr := e.serverRunTest(step)
	if status == "Passed" {
		fmt.Println("Server test passed")
		err := e.activityLogService.CreateActivityLog(
			step.ExecutionStep.ExecutionID,
			step.ExecutionStep.ID,
			"INFO",
			fmt.Sprintf("Server working well!"),
		)
		if err != nil {
			fmt.Println("Error creating activity log" + err.Error())
			return err
		}
		return nil
	} else {
		fmt.Println("Server test failed")
		fmt.Println("Error: ", stderr)

		//Update in DB in execution step
		err := e.executionStepService.UpdateExecutionStepResponse(
			step.ExecutionStep,
			map[string]interface{}{"error": stderr},
			"SUCCESS",
		)
		if err != nil {
			fmt.Printf("Error updating execution step: %s\n", err.Error())
			return fmt.Errorf("%w: %v", steps.ErrReiterate, err)
		}

		err = e.activityLogService.CreateActivityLog(
			step.ExecutionStep.ExecutionID,
			step.ExecutionStep.ID,
			"ERROR",
			fmt.Sprintf("Server test failed: %s", stderr),
		)
		if err != nil {
			fmt.Println("Error creating activity log" + err.Error())
			return fmt.Errorf("%w: %v", steps.ErrReiterate, err)
		}
		err = e.activityLogService.CreateActivityLog(
			step.ExecutionStep.ExecutionID,
			step.ExecutionStep.ID,
			"INFO",
			fmt.Sprintf("Server test failed fixing the issue..."),
		)

		if err != nil {
			fmt.Println("Error creating activity log" + err.Error())
			return fmt.Errorf("%w: %v", steps.ErrReiterate, err)
		}

		return fmt.Errorf("%w: %v", steps.ErrReiterate, err)
	}
}

func (e *FlaskServerStartTestExecutor) getFlaskServerURL() string {
	return "http://127.0.0.1:5000/"
}

func (e *FlaskServerStartTestExecutor) getFlaskServerAppFileName() string {
	return "app.py"
}

// serverRunTest runs the server test.
func (e *FlaskServerStartTestExecutor) serverRunTest(step steps.ServerStartTestStep) (string, string) {
	projectDir := config.WorkspaceWorkingDirectory() + "/" + step.Project.HashID
	appPath := projectDir + "/" + e.getFlaskServerAppFileName()
	serverURL := e.getFlaskServerURL()
	timeout := 60 * time.Second
	dependencyStatus, dependencyError := e.executeDependencies(projectDir)
	if dependencyStatus == "Dependency_Error" {
		return "Failed", dependencyError
	}
	serverProcess, stderr, err := e.startFlaskServer(appPath, projectDir)
	fmt.Println("Server Process: ", serverProcess)
	fmt.Println("Error: ", err)
	fmt.Println("Stderr: ", stderr)
	fmt.Println("Server process started successfully")
	fmt.Println("Server process: ", serverProcess)
	fmt.Println("Checking for server start in stderr: ", strings.Contains(stderr.String(), "Running on http://127.0.0.1"))
	if err != nil || (stderr.String() != "" && !strings.Contains(stderr.String(), "Running on http://127.0.0.1")) {
		err := serverProcess.Process.Kill()
		if err != nil {
			fmt.Printf("Error killing Flask server process: %s\n", err.Error())
			return "", ""
		}
		fmt.Printf("Error starting Flask server: %s\n", stderr.String())
		return "Failed", stderr.String()
	}
	serverRunning, response, checkServerStatusError := e.checkServerStatus(serverURL, timeout)
	if serverRunning {
		fmt.Println("Server is running!")
		fmt.Println("Response: ", response)
		fmt.Println("Check Error: ", checkServerStatusError)
		fmt.Println("Server Process: ", serverProcess)
		err := serverProcess.Process.Kill()
		if err != nil {
			fmt.Printf("Error killing Flask server process: %s\n", err.Error())
			return "", ""
		}
		fmt.Println("Server process killed successfully")
		return "Passed", response
	}
	fmt.Println("Server Process: ", serverProcess)
	stderrPipe, _ := serverProcess.StderrPipe()
	fmt.Println("stderr pipe: ", stderrPipe)
	err = serverProcess.Process.Kill()
	if err != nil {
		fmt.Printf("Error killing Flask server process: %s\n", err.Error())
		return "", ""
	}

	if stderrPipe != nil {
		output := make([]byte, 1024)
		n, err1 := stderrPipe.Read(output)
		if err1 != nil {
			errorMessage := fmt.Sprintf("Error obtaining stderr pipe: %s\n", err1.Error())
			fmt.Print(errorMessage)
			//return "Failed", errorMessage
		}
		errorMessage := fmt.Sprintf("Error starting Flask server: %s\n", string(output[:n]))
		if checkServerStatusError != nil {
			errorMessage = fmt.Sprintf("%s\n%s", errorMessage, checkServerStatusError.Error())
		}
		return "Failed", errorMessage
	}
	if checkServerStatusError != nil {
		fmt.Println("Finally returning check error")
		return "Failed", checkServerStatusError.Error()
	}
	return "Failed", "Unknown error occurred while starting the server and hitting the endpoint for testing"
}

// startFlaskServer starts the Flask server.
func (e *FlaskServerStartTestExecutor) startFlaskServer(appPath string, workDir string) (*exec.Cmd, *bytes.Buffer, error) {
	venvPath := filepath.Join(workDir, ".venv") // Assuming the virtual environment directory is named .venv
	venvBin := filepath.Join(venvPath, "bin")
	pythonPath := filepath.Join(venvBin, "python") // This should point to the Python executable in the virtual environment

	// Create the PATH environment variable to use the virtual environment.
	newPath := fmt.Sprintf("PATH=%s:%s", venvBin, os.Getenv("PATH"))

	var stderrBuf bytes.Buffer
	var stdOutputBuf bytes.Buffer
	fmt.Printf("Starting Flask server using command: %s %s\n", pythonPath, appPath)

	stdout := e.CheckPythonVersion(workDir, newPath)
	fmt.Println("Which Python Output: ", string(stdout))

	cmd, err := RunServer(appPath, workDir, pythonPath, newPath, &stdOutputBuf, &stderrBuf)
	fmt.Println("Error: ", err)
	if err != nil {
		errorMessage := fmt.Sprintf("Error starting Flask server: %s\n", stderrBuf.String())
		fmt.Print(errorMessage)
		return nil, &stderrBuf, err
	}
	fmt.Println("Here waiting for server to start...")
	time.Sleep(20 * time.Second)
	fmt.Println("Here after waiting for server to start...")

	fmt.Println("STDOUT: ______________ ")
	fmt.Println(stdOutputBuf.String())
	fmt.Println("STDERR: __________ ")
	fmt.Println(stderrBuf.String())
	fmt.Println("Cmd.Err : ", cmd.Err)
	return cmd, &stderrBuf, nil
}

func RunServer(appPath string, workDir string, pythonPath string, newPath string, stdOutputBuf *bytes.Buffer, stderrBuf *bytes.Buffer) (*exec.Cmd, error) {
	cmd := exec.Command(pythonPath, appPath)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), newPath) // Set the environment for the command
	//cmd.Env = env
	//cmd.Stdout = os.Stdout
	cmd.Stdout = stdOutputBuf
	cmd.Stderr = stderrBuf
	fmt.Printf("Starting Flask server...\n")
	err := cmd.Start()
	return cmd, err
}

func (e *FlaskServerStartTestExecutor) CheckPythonVersion(workDir string, newPath string) []byte {
	// Prepare the command to check the Python path
	cmd1 := exec.Command("which", "python")
	cmd1.Dir = workDir // Set the directory to the working directory

	cmd1.Env = append(os.Environ(), newPath) // Include the modified PATH in the environment
	e.logger.Debug("DIR : ")
	fmt.Println(cmd1.Dir)
	e.logger.Debug("ENV: ")
	fmt.Println(cmd1.Env)
	// Execute the command and capture the output
	stdout, _ := cmd1.CombinedOutput()
	return stdout
}

func (e *FlaskServerStartTestExecutor) checkServerStatus(url string, timeout time.Duration) (bool, string, error) {
	fmt.Println("Checking server status for URL: ", url)
	client := &http.Client{Timeout: timeout}
	var lastResponse string
	var lastError error
	var lastStatusCode int

	for start := time.Now(); time.Since(start) < timeout; {
		resp, err := client.Get(url)
		fmt.Println("_______________________________________________________")
		fmt.Println("Response: ", resp)
		fmt.Println("Error: ", err)
		fmt.Println("_______________________________________________________")

		if err != nil {
			fmt.Printf("Error: %s\n", err.Error())
			lastError = err
			time.Sleep(1 * time.Second)
			continue
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return false, "", err
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			fmt.Println("Server is running!")
			return true, string(bodyBytes), nil
		}

		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			fmt.Printf("Received 4xx status: %d\n", resp.StatusCode)
			return false, string(bodyBytes), fmt.Errorf("Received status code %d from endpoint %s", resp.StatusCode, url)
		}

		if resp.StatusCode >= 500 {
			fmt.Printf("Received 5xx status: %d\n", resp.StatusCode)
			lastResponse = string(bodyBytes)
			lastStatusCode = resp.StatusCode
			lastError = fmt.Errorf("Received status code %d from endpoint %s", resp.StatusCode, url)
			time.Sleep(1 * time.Second)
			continue
		}

		time.Sleep(1 * time.Second)
	}

	if lastError != nil {
		fmt.Println("Last error: ", lastError.Error())
		fmt.Println("Last response: ", lastResponse)
		return false, lastResponse, fmt.Errorf("Last status code %d from endpoint %s: %w", lastStatusCode, url, lastError)
	}
	return false, "", fmt.Errorf("Timeout reached for endpoint %s", url)
}

// executeDependencies executes commands from terminal.txt.
func (e *FlaskServerStartTestExecutor) executeDependencies(workDir string) (string, string) {
	fmt.Println("Executing dependencies...")
	venvPath := filepath.Join(workDir, ".venv") // Assuming the virtual environment directory is named .venv

	// Ensure the virtual environment is created
	_, err := ensureVenv(workDir, venvPath)
	if err != nil {
		e.logger.Error("Error ensuring virtual environment", zap.Error(err))
		errorMessage := fmt.Sprintf("Error ensuring virtual environment: %s\n", err.Error())
		return "Dependency_Error", errorMessage
	}

	// Create the PATH environment variable to use the virtual environment.
	venvBin := filepath.Join(venvPath, "bin")
	poetryBin := "/opt/poetry/bin"
	newPath := fmt.Sprintf("PATH=%s:%s:%s", venvBin, poetryBin, os.Getenv("PATH"))
	updatedEnv := getUpdateEnvs(workDir, newPath)

	fmt.Println("New Path: ", newPath)
	fmt.Println("Updated Env: ", updatedEnv)

	//Set virtual env
	// Set the VIRTUAL_ENV environment variable to the correct path
	err = os.Setenv("VIRTUAL_ENV", venvPath)
	if err != nil {
		errorMessage := fmt.Sprintf("Error setting VIRTUAL_ENV environment variable: %s\n", err.Error())
		return "Dependency_Error", errorMessage
	}

	//Get virtual environment info
	venvInfoOutput := GetPoetryPathInfo(workDir, updatedEnv, err)
	if err != nil {
		errorMessage := fmt.Sprintf("Error getting virtual environment info: %s\n", err.Error())
		fmt.Print(errorMessage)
		return "Dependency_Error", errorMessage
	}
	fmt.Printf("Virtual environment path for poetry: %s\n", strings.TrimSpace(string(venvInfoOutput)))
	commands, err := e.ReadCommandsFromFile(workDir)
	if err != nil {
		errorMessage := fmt.Sprintf("Error reading terminal.txt: %s\n", err.Error())
		fmt.Print(errorMessage)
		return "Dependency_Error", errorMessage

	}

	for _, command := range commands {
		fmt.Printf("\n_____________Executing command_____________: %s\n", command)
		fmt.Println("Checking ENVs : ")
		for _, env := range updatedEnv {
			fmt.Println("Env: ", env)
		}
		// Execute ls -lah command to see the directory contents
		lsOutput, err := ListOutputInDir(workDir, updatedEnv)
		if err != nil {
			errorMessage := fmt.Sprintf("Error executing 'ls -lah': %s\n", err.Error())
			fmt.Print(errorMessage)
			return "Dependency_Error", errorMessage
		}
		fmt.Println("ls -lah Output:\n", string(lsOutput))
		e.listInstalledPackages(venvBin)

		pythonVersion := e.CheckPythonVersion(workDir, newPath)
		fmt.Println("Which Python Output: ", string(pythonVersion))

		stdout, err := ExecuteTerminalCommand(workDir, command, updatedEnv)
		fmt.Println("Execution Output: ", string(stdout))

		if err != nil {
			fmt.Println("Error: ", err)
			if strings.Contains(string(stdout), "alembic") &&
				!strings.Contains(string(stdout), "Error") &&
				!strings.Contains(string(stdout), "ERROR") &&
				strings.Contains(string(stdout), "INFO") {
				fmt.Println("Alembic specific case, assuming it worked well")
				continue
			}
			// Check if the directory has a database present and the command is 'flask db init'
			if e.directoryHasDatabase(workDir) && strings.Contains(command, "flask db init") {
				fmt.Println("Database already initialized, assuming it worked well")
				continue
			}
			errorMessage := fmt.Sprintf("Error executing command '%s': %s\n", command, string(stdout))
			fmt.Print(errorMessage)
			fmt.Println("GOT ERROR : ", err.Error())
			return "Dependency_Error", errorMessage

		} else {
			if strings.Contains(string(stdout), "FAILED") || strings.Contains(string(stdout), "ERROR") {
				errorMessage := fmt.Sprintf("Command '%s' failed with output: %s\n", command, string(stdout))
				fmt.Print(errorMessage)
				return "Dependency_Error", errorMessage

			} else {
				fmt.Println("Executed command successfully")
			}
		}
	}

	// Install dependencies using poetry
	err = PoetryInstall(workDir, err)
	if err != nil {
		errorMessage := fmt.Sprintf("Error creating virtual environment: %s\n", err.Error())
		fmt.Print(errorMessage)
		return "Dependency_Error", errorMessage
	}

	fmt.Println("All commands executed successfully")
	return "Passed", ""
}

func getUpdateEnvs(workDir string, newPath string) []string {
	envs := os.Environ()
	var updatedEnv []string
	for _, env := range envs {
		if strings.HasPrefix(env, "PATH=") {
			continue
		} else {
			updatedEnv = append(updatedEnv, env)
		}
	}
	updatedEnv = append(updatedEnv, newPath, "PYTHONPATH="+workDir)
	return updatedEnv
}

func ExecuteTerminalCommand(workDir string, command string, updatedEnv []string) ([]byte, error) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Env = updatedEnv
	cmd.Dir = workDir
	stdout, err := cmd.CombinedOutput()
	return stdout, err
}

func PoetryInstall(workDir string, err error) error {
	cmd := exec.Command("poetry", "install")
	cmd.Dir = workDir
	err = cmd.Run()
	return err
}

func ListOutputInDir(workDir string, env []string) ([]byte, error) {
	fmt.Println("Executing 'ls -lah' command to see the directory contents...")
	lsCmd := exec.Command("ls", "-lah")
	lsCmd.Dir = workDir
	lsCmd.Env = env

	lsOutput, err := lsCmd.CombinedOutput()
	return lsOutput, err
}

func ChangeDirectoryOwnership(venvPath string, err error) error {
	fmt.Println("Changing ownership of .venv directory to coder...")
	cmd := exec.Command("sudo", "chown", "-R", "coder:coder", venvPath)
	err = cmd.Run()
	return err
}

func GetPoetryPathInfo(workDir string, envs []string, err error) []byte {
	venvInfoCmd := exec.Command("poetry", "env", "info", "--path")
	venvInfoCmd.Dir = workDir
	venvInfoCmd.Env = envs
	venvInfoOutput, err := venvInfoCmd.CombinedOutput()
	return venvInfoOutput
}

func SetPoetryConfigVenvInProject(workDir string, env []string, err error) error {
	cmd := exec.Command("poetry", "config", "virtualenvs.in-project", "true")
	cmd.Dir = workDir
	cmd.Env = env
	fmt.Printf("Setting environment variable in %s to use in-project virtual environment %s\n", cmd.Env, cmd.Dir)
	err = cmd.Run()
	return err
}

func SetPoetryVenvInProject() error {
	err := os.Setenv("POETRY_VIRTUALENVS_IN_PROJECT", "true")
	fmt.Println("Setting environment variable to use in-project virtual environment")
	return err
}

func ensureVenv(workDir string, venvPath string) (string, error) {
	if _, err := os.Stat(venvPath); os.IsNotExist(err) {
		fmt.Println("Virtual environment does not exist. Creating virtual environment...")
		errorMessage, err := createVenv(workDir)
		if err != nil {
			return errorMessage, err
		}
	}
	return "", nil
}

func createVenv(workDir string) (string, error) {
	cmd := exec.Command("python3", "-m", "venv", ".venv")
	cmd.Dir = workDir
	if err := cmd.Run(); err != nil {
		errorMessage := fmt.Sprintf("Error creating virtual environment: %s\n", err.Error())
		fmt.Print(errorMessage)
		return "Dependency_Error", nil
	}
	return "", nil
}

func (e *FlaskServerStartTestExecutor) listInstalledPackages(venvBin string) {
	fmt.Println("Listing installed packages...")
	cmd := exec.Command(filepath.Join(venvBin, "pip"), "list")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error listing installed packages: %s\n", err.Error())
		return
	}
	fmt.Printf("Installed packages:\n%s\n", string(output))
}

func (e *FlaskServerStartTestExecutor) getFlaskTerminalFileName() string {
	return "terminal.txt"
}

func (e *FlaskServerStartTestExecutor) ReadCommandsFromFile(workingDir string) ([]string, error) {
	terminalFilePath := workingDir + "/" + e.getFlaskTerminalFileName()
	absTerminalFilePath, err := filepath.Abs(terminalFilePath)
	if err != nil {
		return nil, fmt.Errorf("error getting absolute path: %w", err)
	}

	file, err := os.Open(absTerminalFilePath)
	if err != nil {
		return nil, fmt.Errorf("error opening terminal.txt: %w", err)
	}
	defer file.Close()

	var commands []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		command := scanner.Text()
		if strings.TrimSpace(command) != "" &&
			!strings.HasPrefix(command, "python") &&
			!strings.HasPrefix(command, "source") &&
			!strings.HasPrefix(command, "flask run --host=") {
			commands = append(commands, command)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading terminal.txt: %w", err)
	}

	return commands, nil
}

func (e *FlaskServerStartTestExecutor) directoryHasDatabase(workDir string) bool {
	dbFilePath := workDir + "/instance" + "/" + "app.db"
	if _, err := os.Stat(dbFilePath); err == nil {
		return true
	}
	return false
}
