package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kujiu27/nemotron-healer-go/internal/cli"
)

func main() {
	// Graceful shutdown on SIGINT / SIGTERM
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\nReceived termination signal. Exiting cleanly...")
		os.Exit(0)
	}()

	if err := cli.RootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
