package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kujiu27/nemotron-healer-go/internal/cli"
)

func main() {
	// Fail-closed shutdown on SIGINT (130) / SIGTERM (143).
	// Exiting with 0 on abort masks killed jobs as successful in CI pipelines.
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-c
		fmt.Fprintf(os.Stderr, "\nReceived termination signal (%s). Exiting...\n", sig)
		if sig == syscall.SIGTERM {
			os.Exit(143)
		}
		os.Exit(130)
	}()
	if err := cli.RootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
