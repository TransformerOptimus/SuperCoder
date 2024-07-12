package utils

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

func CheckIfWorkspaceExists(workspaceId string) (bool, error) {
	_, err := os.Stat("/workspaces/" + workspaceId)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func CheckIfFrontendWorkspaceExists(storyHashId, workspaceId string) (bool, error) {
	_, err := os.Stat("/workspaces/stories/" + workspaceId + "/" + storyHashId)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func SudoRsyncFolders(src, dest string) error {
    cmd := exec.Command("rsync", "-av", src, dest)
    
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    
    err := cmd.Run()
    if err != nil {
        return fmt.Errorf("rsync error: %v\nStdout: %s\nStderr: %s", err, stdout.String(), stderr.String())
    }
    
    return nil
}

func RsyncFolders(src string, dest string) error {
	cmd := exec.Command("rsync", "-av", src, dest)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func ChownRWorkspace(user string, group string, workspacePath string) error {
	cmd := exec.Command(
		"chown",
		"-R",
		fmt.Sprintf("%s:%s", user, group),
		workspacePath,
	)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}
