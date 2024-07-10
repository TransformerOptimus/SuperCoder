package main

import (
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

func main() {
	c := exec.Command("bash")
	f, err := pty.Start(c)
	if err != nil {
		panic(err)
	}

	go func() {
		f.Write([]byte("ls\n"))
		f.Write([]byte{4}) // EOT
	}()
	//fmt.Println(*f)
	io.Copy(os.Stdout, f)
}
