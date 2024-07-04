package impl

import (
	"ai-developer/app/config"
	"ai-developer/app/services"
	"ai-developer/app/workflow_executors/step_executors/steps"
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

type DjangoServerStartTestExecutor struct {
	executionStepService *services.ExecutionStepService
	activityLogService   *services.ActivityLogService
	logger               *zap.Logger
}

func NewDjangoServerStartTestExecutor(
	executionStepService *services.ExecutionStepService,
	activityLogService *services.ActivityLogService,
	logger *zap.Logger,
) *DjangoServerStartTestExecutor {
	return &DjangoServerStartTestExecutor{
		executionStepService: executionStepService,
		activityLogService:   activityLogService,
		logger:               logger,
	}
}

func (e DjangoServerStartTestExecutor) Execute(step steps.ServerStartTestStep) error {
	fmt.Printf("Executing Django Server Start Test Step: %s\n", step.StepName())

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

func (e *DjangoServerStartTestExecutor) getDjangoServerURL() string {
	return "http://127.0.0.1:5000/"
}

func (e *DjangoServerStartTestExecutor) getDjangoServerAppFileName() string {
	return "manage.py"
}

// serverRunTest runs the server test.
func (e *DjangoServerStartTestExecutor) serverRunTest(step steps.ServerStartTestStep) (string, string) {
	projectDir := config.WorkspaceWorkingDirectory() + "/" + step.Project.HashID
	appPath := projectDir + "/" + e.getDjangoServerAppFileName()
	serverURL := e.getDjangoServerURL()
	timeout := 60 * time.Second
	dependencyStatus, dependencyError := e.executeDependencies(projectDir)
	if dependencyStatus == "Dependency_Error" {
		return "Failed", dependencyError
	}
	serverProcess, stdout, stderr, err := e.startDjangoServer(appPath, projectDir)
	fmt.Println("Server Process: ", serverProcess)
	fmt.Println("Error: ", err)
	fmt.Println("Stderr: ", stderr)
	fmt.Println("Stdout: ", stdout)
	fmt.Println("Server process started successfully")
	fmt.Println("Server process: ", serverProcess)
	fmt.Println("Checking for server start in stderr: ", strings.Contains(stdout.String(), "Starting development server at"))
	if err != nil || (stderr.String() != "" && !strings.Contains(stdout.String(), "Starting development server at")) {
		if serverProcess != nil && serverProcess.Process != nil {
			err := serverProcess.Process.Kill()
			if err != nil {
				fmt.Printf("Error killing Django server process: %s\n", err.Error())
				return "", ""
			}
		} else {
			fmt.Println("Server process is nil, cannot kill the process.")
		}
		e.cleanupPort(5000)
		fmt.Printf("Error starting Django server: %s\n", stderr.String())
		return "Failed", stderr.String()
	}

	server_status, err_msg, response_body := e.checkServerStatus(serverURL, timeout)
	if server_status{
		fmt.Println("Server is running!")
		if serverProcess != nil && serverProcess.Process != nil {
			err := serverProcess.Process.Kill()
			if err != nil {
				fmt.Printf("Error killing Django server process: %s\n", err.Error())
				return "", ""
			}
		}
		fmt.Println("Server process killed successfully")
		return "Passed", ""
	} else {
		e.cleanupPort(5000)
		return "Failed", err_msg+string(response_body)
	}
}

func (e *DjangoServerStartTestExecutor) startDjangoServer(appPath string, workDir string) (*exec.Cmd, *bytes.Buffer, *bytes.Buffer, error) {
    venvPath := filepath.Join(workDir, ".venv") // Assuming the virtual environment directory is named .venv
    venvBin := filepath.Join(venvPath, "bin")
    pythonPath := filepath.Join(venvBin, "python") // This should point to the Python executable in the virtual environment

    // Create the PATH environment variable to use the virtual environment.
    newPath := fmt.Sprintf("PATH=%s:%s", venvBin, os.Getenv("PATH"))

    var stderrBuf bytes.Buffer
    var stdOutputBuf bytes.Buffer
    fmt.Printf("Starting Django server using command: %s %s\n", pythonPath, appPath)

    stdout := e.CheckPythonVersion(workDir, newPath)
    fmt.Println("Which Python Output: ", string(stdout))

    cmd, err := RunDjangoServer(appPath, workDir, pythonPath, newPath, &stdOutputBuf, &stderrBuf)
    fmt.Println("Error: ", err)
    if err != nil {
        errorMessage := fmt.Sprintf("Error starting Django server: %s\n", stderrBuf.String())
        fmt.Print(errorMessage)
        return nil, nil, &stderrBuf, err
    }
    fmt.Println("Here waiting for server to start...")
    time.Sleep(20 * time.Second)
    fmt.Println("Here after waiting for server to start...")

    fmt.Println("STDOUT: ______________ ")
    fmt.Println(stdOutputBuf.String())
    fmt.Println("STDERR: __________ ")
    fmt.Println(stderrBuf.String())
    fmt.Println("Cmd.Err : ", cmd.Err)
    return cmd, &stdOutputBuf, &stderrBuf, nil
}

func RunDjangoServer(appPath string, workDir string, pythonPath string, newPath string, stdOutputBuf *bytes.Buffer, stderrBuf *bytes.Buffer) (*exec.Cmd, error) {
    commands := []string{
        fmt.Sprintf("%s %s makemigrations", pythonPath, appPath),
        fmt.Sprintf("%s %s migrate", pythonPath, appPath),
        fmt.Sprintf("%s %s runserver 0.0.0.0:5000", pythonPath, appPath),
    }

    for i, command := range commands {
        cmd := exec.Command("sh", "-c", command)
        cmd.Dir = workDir
        cmd.Env = append(os.Environ(), newPath)
        
        if i < len(commands)-1 { // For the first two commands
            var combinedOutput bytes.Buffer
            cmd.Stdout = &combinedOutput
            cmd.Stderr = &combinedOutput
            fmt.Printf("Running: %s\n", command)
            err := cmd.Start()
            if err != nil {
                fmt.Printf("failed to start command: %v\n", err)
                return cmd, err
            }
            err = cmd.Wait()
            if err != nil {
                fmt.Printf("command failed: %v\n", err)
                stderrBuf.Write(combinedOutput.Bytes()) // Capture the error output
                return cmd, err
            }
            stdOutputBuf.Write(combinedOutput.Bytes())
        } else { // For the last command to run the server
            cmd.Stdout = stdOutputBuf
            cmd.Stderr = stderrBuf
            fmt.Println("Starting Django server...")
            err := cmd.Start()
            if err != nil {
                fmt.Printf("failed to start Django server: %v\n", err)
                return cmd, err
            }
            return cmd, nil
        }
    }
    return nil, nil
}

func (e *DjangoServerStartTestExecutor) cleanupPort(port int) {
    fmt.Printf("Cleaning up any processes using port %d\n", port)
    cmd := exec.Command("sh", "-c", fmt.Sprintf("fuser -k %d/tcp", port))
    err := cmd.Run()
    if err != nil {
        fmt.Printf("Error cleaning up port %d: %v\n", port, err)
    } else {
        fmt.Printf("Successfully cleaned up port %d\n", port)
    }
}


func (e *DjangoServerStartTestExecutor) CheckPythonVersion(workDir string, newPath string) []byte {
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

// checkServerStatus checks if the server is running.
func (e *DjangoServerStartTestExecutor) checkServerStatus(url string, timeout time.Duration) (bool, string, []byte) {
    fmt.Println("Checking server status...")
    client := &http.Client{Timeout: timeout}
    
    for start := time.Now(); time.Since(start) < timeout; {
        resp, err := client.Get(url)
        fmt.Println("_______________________________________________________")
        fmt.Println("Response: ", resp)
        
        if err != nil {
            fmt.Println("Error occurred:", err)
            return false, "Error occurred: " + err.Error(), nil
        }
        
        if resp.StatusCode == http.StatusOK {
            fmt.Println("Server is running!")
            return true, "", nil
        }
        
        if resp.Body != nil {
            defer resp.Body.Close()
            body, err := io.ReadAll(resp.Body)
            if err != nil {
                fmt.Println("Error reading response body:", err)
                return false, "Error reading response body: " + err.Error(), nil
            }
            return false, "Server responded with status code: " + strconv.Itoa(resp.StatusCode), body
        }
        time.Sleep(1 * time.Second)
    }
    return false, "Request timed out", nil
}

// executeDependencies executes commands from terminal.txt.
func (e *DjangoServerStartTestExecutor) executeDependencies(workDir string) (string, string) {
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

func (e *DjangoServerStartTestExecutor) listInstalledPackages(venvBin string) {
	fmt.Println("Listing installed packages...")
	cmd := exec.Command(filepath.Join(venvBin, "pip"), "list")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error listing installed packages: %s\n", err.Error())
		return
	}
	fmt.Printf("Installed packages:\n%s\n", string(output))
}

func (e *DjangoServerStartTestExecutor) getFlaskTerminalFileName() string {
	return "terminal.txt"
}

func (e *DjangoServerStartTestExecutor) ReadCommandsFromFile(workingDir string) ([]string, error) {
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

func (e *DjangoServerStartTestExecutor) directoryHasDatabase(workDir string) bool {
	dbFilePath := workDir + "/instance" + "/" + "app.db"
	if _, err := os.Stat(dbFilePath); err == nil {
		return true
	}
	return false
}
