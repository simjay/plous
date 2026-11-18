package debugging

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/simjay/plous/skills"
	"golang.org/x/sys/unix"
)

const skillMarker = "<!-- Managed by plous setup. -->\n"
const serverName = "plous_debug"

func (client setupClient) install(c configuration, output io.Writer) error {
	home := c.home
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return err
		}
	}
	home, err := filepath.Abs(home)
	if err != nil {
		return err
	}
	executable, err := setupExecutable()
	if err != nil {
		return err
	}
	configPath, skillPath := client.paths(home, c.home == "")
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	// The directory stays locked across the read and atomic replacement of its config.
	lock, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) {
			return errors.New("another setup is updating this configuration; retry setup")
		}
		return err
	}
	configPath, err = client.configFile(configPath, executable)
	if err != nil {
		return err
	}
	oldConfig, err := readInstallFile(configPath)
	if err != nil {
		return err
	}
	monitorPath := filepath.Join(filepath.Dir(filepath.Dir(skillPath)), "plous-monitor", "SKILL.md")
	type installSkill struct {
		path, name, body string
		old              []byte
	}
	installedSkills := []installSkill{
		{path: skillPath, name: "plous-debug", body: skills.DebugSkill},
		{path: monitorPath, name: "plous-monitor", body: skills.MonitorSkill},
	}
	for i := range installedSkills {
		skill := &installedSkills[i]
		skill.old, err = readInstallFile(skill.path)
		if err != nil {
			return err
		}
		if len(skill.old) > 0 && !bytes.Equal(skill.old, []byte(skill.body)) && !bytes.Contains(skill.old, []byte("\n---\n\n"+skillMarker)) {
			return fmt.Errorf("%s skill already exists and is not managed by Plous", skill.name)
		}
	}
	newConfig, err := client.mergeConfig(oldConfig, executable, client.launchArgs(c))
	if err != nil {
		return err
	}
	for _, skill := range installedSkills {
		// Keep YAML frontmatter at the start for clients that require it.
		body := strings.Replace(skill.body, "\n---\n", "\n---\n\n"+skillMarker, 1)
		if err := writeInstallFile(skill.path, skill.old, []byte(body)); err != nil {
			return err
		}
	}
	if err = writeInstallFile(configPath, oldConfig, newConfig); err != nil {
		return fmt.Errorf("skill installed, but MCP configuration could not be saved: %w", err)
	}
	fmt.Fprintf(output, `Configured plous-debug for %s.
Executable: %s
Skill: %s
Monitor skill: %s
MCP server plous_debug: %s
Target: follows the agent process's current kubecontext until you select another target
Exec enabled: %t
Temporary diagnostics enabled: %t

Restart or reconnect the agent, then use plous-debug and describe what broke.
Guide: https://plous.ai/docs/agent-skills/clients/%s/
`, client.kind.Label(), executable, skillPath, monitorPath, configPath, c.policy.Exec, c.policy.Image != "", client.command)
	return nil
}

func setupExecutable() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	// Package managers can replace the target of a stable launch path on upgrade.
	for _, name := range []string{"plous", os.Args[0]} {
		path, err := exec.LookPath(name)
		if err == nil && sameExecutable(executable, path) {
			return filepath.Abs(path)
		}
	}
	return filepath.Abs(executable)
}

func sameExecutable(left, right string) bool {
	leftInfo, err := os.Stat(left)
	if err != nil {
		return false
	}
	rightInfo, err := os.Stat(right)
	return err == nil && os.SameFile(leftInfo, rightInfo)
}

func readInstallFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("installation target must be a regular file: %s", path)
	}
	return os.ReadFile(path)
}

func writeInstallFile(path string, old, data []byte) error {
	if bytes.Equal(old, data) {
		current, err := readInstallFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, data) {
			return fmt.Errorf("installation target changed during setup: %s; retry setup", path)
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() && info.Mode().Perm() == 0600 {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "plous-install-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	current, err := readInstallFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, old) && !bytes.Equal(current, data) {
		return fmt.Errorf("installation target changed during setup: %s; retry setup", path)
	}
	return os.Rename(file.Name(), path)
}
