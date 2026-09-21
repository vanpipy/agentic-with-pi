package tui

import (
	"fmt"
	"os/exec"
	"runtime"
)

func CopyToClipboard(text string) error {
	if text == "" {
		return fmt.Errorf("empty text")
	}

	var candidates [][]string
	switch runtime.GOOS {
	case "darwin":
		candidates = [][]string{{"pbcopy"}}
	case "windows":
		candidates = [][]string{{"clip"}}
	default:
		candidates = [][]string{
			{"wl-copy"},
			{"xclip", "-selection", "clipboard"},
			{"xsel", "--clipboard", "--input"},
		}
	}

	for _, cmd := range candidates {
		if len(cmd) == 0 {
			continue
		}
		if _, err := exec.LookPath(cmd[0]); err != nil {
			continue
		}
		c := exec.Command(cmd[0], cmd[1:]...)
		stdin, err := c.StdinPipe()
		if err != nil {
			continue
		}
		if err := c.Start(); err != nil {
			continue
		}
		if _, err := stdin.Write([]byte(text)); err != nil {
			_ = c.Wait()
			continue
		}
		if err := stdin.Close(); err != nil {
			_ = c.Wait()
			continue
		}
		if err := c.Wait(); err != nil {
			continue
		}
		return nil
	}
	return fmt.Errorf("no clipboard command available")
}