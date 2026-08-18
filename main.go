package main

import "github.com/metronome-industries/quikstrate/cmd"

// version is set via -ldflags "-X main.version=..." at build time (see .goreleaser.yaml).
var version = "dev"

func main() {
	cmd.Execute(version)
}
