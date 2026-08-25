package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

type DockerConfigJSON struct {
	Auths map[string]DockerAuthEntry `json:"auths"`
}

type DockerAuthEntry struct {
	Auth     string `json:"auth"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type Credentials struct {
	Registry string
	Username string
	Password string
}

func ParseDockerCredentials(raw []byte) (Credentials, error) {
	var cfg DockerConfigJSON
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Credentials{}, fmt.Errorf("parse docker config: %w", err)
	}

	for registry, entry := range cfg.Auths {
		username, password, err := decodeDockerAuth(entry)
		if err != nil {
			return Credentials{}, fmt.Errorf("decode auth for registry %q: %w", registry, err)
		}
		return Credentials{Registry: registry, Username: username, Password: password}, nil
	}
	return Credentials{}, fmt.Errorf("docker config contains no registry auths")
}

func decodeDockerAuth(entry DockerAuthEntry) (string, string, error) {
	if entry.Username != "" || entry.Password != "" {
		return entry.Username, entry.Password, nil
	}

	if entry.Auth == "" {
		return "", "", fmt.Errorf("empty auth field")
	}

	decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
	if err != nil {
		return "", "", fmt.Errorf("base64 decode auth: %w", err)
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("decoded auth is not username:password")
	}

	return parts[0], parts[1], nil
}
