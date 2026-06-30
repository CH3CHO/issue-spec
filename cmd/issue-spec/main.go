package main

import (
	"os"

	"github.com/higress-group/issue-spec/internal/commands"
)

func main() {
	os.Exit(commands.Execute(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
