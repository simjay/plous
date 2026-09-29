package main

import (
	_ "embed"
	"os"
	"strings"

	"github.com/simjay/plous/internal/app"
)

//go:embed VERSION
var versionFile string

func main() {
	os.Exit(app.Run(strings.TrimSpace(versionFile)))
}
