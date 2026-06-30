package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	keyring "github.com/zalando/go-keyring"
)

const (
	serviceName = "issue-spec"
)

var ErrNoToken = errors.New("no issue-spec token is available")

type Token struct {
	Value  string   `json:"-"`
	Source string   `json:"source"`
	User   string   `json:"user,omitempty"`
	Scopes []string `json:"scopes,omitempty"`
	Host   string   `json:"host"`
}

type StoredCredential struct {
	Token string `json:"token"`
}

type credentialFile struct {
	Hosts map[string]StoredCredential `json:"hosts"`
}

func ResolveToken(_ context.Context, host string) (Token, error) {
	host = NormalizeHost(host)
	for _, envName := range []string{"ISSUE_SPEC_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"} {
		if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
			return Token{Value: value, Source: "env:" + envName, Host: host}, nil
		}
	}

	if value, err := keyring.Get(serviceName, host); err == nil && strings.TrimSpace(value) != "" {
		return Token{Value: strings.TrimSpace(value), Source: "keyring", Host: host}, nil
	}

	creds, err := readCredentialFile()
	if err != nil {
		return Token{}, err
	}
	if stored, ok := creds.Hosts[host]; ok && strings.TrimSpace(stored.Token) != "" {
		return Token{Value: strings.TrimSpace(stored.Token), Source: "config", Host: host}, nil
	}

	return Token{Host: host}, ErrNoToken
}

func StoreToken(_ context.Context, host, token string, insecureStorage bool) (string, error) {
	host = NormalizeHost(host)
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("token is empty")
	}

	if !insecureStorage {
		if err := keyring.Set(serviceName, host, token); err != nil {
			return "", fmt.Errorf("store token in OS keyring for %s: %w; rerun with --insecure-storage to use explicit plaintext fallback", host, err)
		}
		return "keyring", nil
	}

	creds, err := readCredentialFile()
	if err != nil {
		return "", err
	}
	if creds.Hosts == nil {
		creds.Hosts = map[string]StoredCredential{}
	}
	creds.Hosts[host] = StoredCredential{Token: token}
	if err := writeCredentialFile(creds); err != nil {
		return "", err
	}
	return "config", nil
}

func DeleteToken(_ context.Context, host string) error {
	host = NormalizeHost(host)
	var errs []error
	if err := keyring.Delete(serviceName, host); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		errs = append(errs, err)
	}

	creds, err := readCredentialFile()
	if err != nil {
		errs = append(errs, err)
	} else if _, ok := creds.Hosts[host]; ok {
		delete(creds.Hosts, host)
		if err := writeCredentialFile(creds); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func EnvTokenActive() string {
	for _, envName := range []string{"ISSUE_SPEC_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"} {
		if strings.TrimSpace(os.Getenv(envName)) != "" {
			return envName
		}
	}
	return ""
}

func NormalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return "github.com"
	}
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimSuffix(host, "/")
	return host
}

func ConfigDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("ISSUE_SPEC_CONFIG_DIR")); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "issue-spec"), nil
}

func credentialPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.json"), nil
}

func readCredentialFile() (credentialFile, error) {
	path, err := credentialPath()
	if err != nil {
		return credentialFile{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return credentialFile{Hosts: map[string]StoredCredential{}}, nil
	}
	if err != nil {
		return credentialFile{}, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return credentialFile{Hosts: map[string]StoredCredential{}}, nil
	}
	var creds credentialFile
	if err := json.Unmarshal(data, &creds); err != nil {
		return credentialFile{}, fmt.Errorf("read issue-spec credentials %s: %w", path, err)
	}
	if creds.Hosts == nil {
		creds.Hosts = map[string]StoredCredential{}
	}
	return creds, nil
}

func writeCredentialFile(creds credentialFile) error {
	path, err := credentialPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
