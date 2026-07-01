package commands

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/higress-group/issue-spec/internal/auth"
	"github.com/higress-group/issue-spec/internal/github"
)

func (a *app) runAuth(ctx context.Context, args []string) int {
	if len(args) == 0 {
		a.errorf("usage: issue-spec auth status|login|logout|token\n")
		return 2
	}
	switch args[0] {
	case "status":
		return a.runAuthStatus(ctx, args[1:])
	case "login":
		return a.runAuthLogin(ctx, args[1:])
	case "logout":
		return a.runAuthLogout(ctx, args[1:])
	case "token":
		return a.runAuthToken(ctx, args[1:])
	default:
		a.errorf("unknown auth command %q\n", args[0])
		return 2
	}
}

func (a *app) runAuthStatus(ctx context.Context, args []string) int {
	fs := newFlagSet("auth status", a.err)
	host := fs.String("hostname", "github.com", "GitHub hostname")
	jsonOut := fs.Bool("json", false, "write JSON output")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	client, token, err := a.clientFor(ctx, *host)
	if err != nil {
		if *jsonOut {
			return a.outputJSON(authErrorResult(token, err))
		}
		a.errorf("not authenticated for %s: %v\n", auth.NormalizeHost(*host), err)
		return 1
	}
	user, scopes, err := client.GetUser(ctx)
	if err != nil {
		if *jsonOut {
			return a.outputJSON(authErrorResult(token, err))
		}
		a.errorf("validate token for %s from %s: %v\n", token.Host, token.Source, err)
		return 1
	}
	token.User = user.Login
	token.Scopes = scopes
	if *jsonOut {
		return a.outputJSON(map[string]any{"ok": true, "auth": token, "backend": token.Backend})
	}
	fmt.Fprintf(a.out, "github host: %s\nuser: %s\ntoken source: %s\n", token.Host, token.User, token.Source)
	if token.Backend != nil {
		fmt.Fprintf(a.out, "github backend: %s (%s)\n", token.Backend.Name, token.Backend.SelectionSource)
	}
	if len(token.Scopes) > 0 {
		fmt.Fprintf(a.out, "scopes: %s\n", strings.Join(token.Scopes, ", "))
	}
	return 0
}

func (a *app) runAuthLogin(ctx context.Context, args []string) int {
	fs := newFlagSet("auth login", a.err)
	host := fs.String("hostname", "github.com", "GitHub hostname")
	withToken := fs.Bool("with-token", false, "read token from stdin")
	insecure := fs.Bool("insecure-storage", false, "store token in issue-spec plaintext config when keyring is unavailable or undesired")
	jsonOut := fs.Bool("json", false, "write JSON output")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !*withToken {
		a.errorf("auth login currently requires --with-token\n")
		return 2
	}
	data, err := io.ReadAll(a.in)
	if err != nil {
		a.errorf("read token from stdin: %v\n", err)
		return 1
	}
	tokenValue := strings.TrimSpace(string(data))
	if tokenValue == "" {
		a.errorf("stdin token is empty\n")
		return 1
	}
	hostName := auth.NormalizeHost(*host)
	user, scopes, err := github.NewClient(hostName, tokenValue).GetUser(ctx)
	if err != nil {
		a.errorf("validate token for %s: %v\n", hostName, err)
		return 1
	}
	source, err := auth.StoreToken(ctx, hostName, tokenValue, *insecure)
	if err != nil {
		a.errorf("%v\n", err)
		return 1
	}
	result := map[string]any{"ok": true, "host": hostName, "user": user.Login, "source": source, "scopes": scopes}
	if *jsonOut {
		return a.outputJSON(result)
	}
	if *insecure {
		fmt.Fprintln(a.err, "warning: token stored in issue-spec plaintext config because --insecure-storage was set")
	}
	fmt.Fprintf(a.out, "logged in to %s as %s using %s storage\n", hostName, user.Login, source)
	return 0
}

func (a *app) runAuthLogout(ctx context.Context, args []string) int {
	fs := newFlagSet("auth logout", a.err)
	host := fs.String("hostname", "github.com", "GitHub hostname")
	jsonOut := fs.Bool("json", false, "write JSON output")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	hostName := auth.NormalizeHost(*host)
	err := auth.DeleteToken(ctx, hostName)
	envActive := auth.EnvTokenActive()
	if err != nil {
		a.errorf("logout %s: %v\n", hostName, err)
		return 1
	}
	result := map[string]any{"ok": true, "host": hostName, "env_token_active": envActive}
	if *jsonOut {
		return a.outputJSON(result)
	}
	fmt.Fprintf(a.out, "removed persisted issue-spec token for %s\n", hostName)
	if envActive != "" {
		fmt.Fprintf(a.out, "environment token %s is still active and was not unset\n", envActive)
	}
	return 0
}

func (a *app) runAuthToken(ctx context.Context, args []string) int {
	fs := newFlagSet("auth token", a.err)
	host := fs.String("hostname", "github.com", "GitHub hostname")
	plain := fs.Bool("plain", false, "print token in plain text")
	jsonOut := fs.Bool("json", false, "write JSON output")
	includeToken := fs.Bool("include-token", false, "include token in JSON output")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	selection, err := a.selectBackend(ctx, *host)
	if err != nil {
		token := selection.TokenWithDiagnostics()
		if *jsonOut {
			return a.outputJSON(authErrorResult(token, err))
		}
		if errors.Is(err, auth.ErrNoToken) {
			a.errorf("not authenticated for %s\n", auth.NormalizeHost(*host))
		} else {
			a.errorf("resolve token: %v\n", err)
		}
		return 1
	}
	token := selection.TokenWithDiagnostics()
	if !*plain && !*jsonOut {
		a.errorf("refusing to print token without --plain\n")
		return 2
	}
	if *jsonOut {
		out := map[string]any{"host": token.Host, "source": token.Source, "backend": token.Backend}
		if *includeToken {
			tokenValue, err := a.tokenForSelection(ctx, selection)
			if err != nil {
				return a.outputJSON(authErrorResult(token, err))
			}
			out["token"] = tokenValue
		}
		return a.outputJSON(out)
	}
	tokenValue, err := a.tokenForSelection(ctx, selection)
	if err != nil {
		a.errorf("resolve token: %v\n", err)
		return 1
	}
	fmt.Fprintln(a.out, tokenValue)
	return 0
}

var _ = flag.ContinueOnError

func authErrorResult(token auth.Token, err error) map[string]any {
	result := map[string]any{
		"ok":    false,
		"host":  token.Host,
		"error": err.Error(),
	}
	if token.Source != "" {
		result["source"] = token.Source
	}
	if token.Backend != nil {
		result["backend"] = token.Backend
	}
	return result
}
