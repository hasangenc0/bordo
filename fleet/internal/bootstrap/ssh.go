package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// dialSSH establishes an SSH connection using a private key file.
func dialSSH(cfg Config) (*ssh.Client, error) {
	keyData, err := readKeyFile(cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("reading key %s: %w", cfg.KeyPath, err)
	}

	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}

	sshCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: use known_hosts in production
		Timeout:         30 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	return ssh.Dial("tcp", addr, sshCfg)
}

func readKeyFile(path string) ([]byte, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, path[2:])
	}
	return os.ReadFile(path)
}
