package utils

import (
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

func SudoRsyncFolders(src string, dest string) error {
	cmd := exec.Command("sudo", "rsync", "-av", src, dest)
	err := cmd.Run()
	if err != nil {
		return err
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

func ChownRWorkspace(workspaceId string, user string, group string) error {
	cmd := exec.Command(
		"chown",
		"-R",
		fmt.Sprintf("%s:%s", user, group),
		fmt.Sprintf("/workspaces/%s", workspaceId),
	)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}
