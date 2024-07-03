package utils

import (
	"fmt"
	"os"
	"os/exec"
)

func RunCommand(name string, dir string, arg ...string) error {
	fmt.Println("________________Running command_____________ : ", name, arg, dir)
	cmd := exec.Command(name, arg...)
	cmd.Dir = dir

	// Set the environment variables
	cmd.Env = append(os.Environ(), "PATH="+os.Getenv("PATH"))

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Error running command: ", string(output))
		return fmt.Errorf("failed to run command: %s", err)
	}
	fmt.Println("Command output: ", string(output))
	return nil
}
