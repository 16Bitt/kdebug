package main

import (
	"flag"
	"fmt"
	"strings"
	"time"
)

type flagArray []string

type cliOptions struct {
	namespace     string
	name          string
	resourceType  string
	source        string
	containerName string
	image         string
	entrypoint    []string
	timeout       time.Duration
}

func newCliOptions() *cliOptions {
	entryArgs := &flagArray{}

	ns := flag.String("namespace", "default", "Namespace for resource and debug pod")
	name := flag.String("name", "kdebug-pod", "Name of debugging pod created")
	resourceType := flag.String("type", "deployment", "Resource type to debug")
	source := flag.String("source", "", "Resource name to debug")
	containerName := flag.String("container-name", "", "Container name to target, if set, otherwise uses the first container")
	image := flag.String("image", "", "Image to use, if set")
	timeoutRaw := flag.String("timeout", "30m", "Timeout for the entrypoint. Only used if entrypoint is not overridden.")
	flag.Var(entryArgs, "entry", "Entrypoint executable to execute while connecting a shell (repeat the flag to pass arguments)")
	flag.Parse()

	timeout, err := time.ParseDuration(*timeoutRaw)
	if err != nil {
		panic(err)
	}

	return &cliOptions{
		namespace:     *ns,
		name:          *name,
		resourceType:  *resourceType,
		source:        *source,
		entrypoint:    entryArgs.toStringArr(),
		timeout:       timeout,
		containerName: *containerName,
		image:         *image,
	}
}

func (co *cliOptions) validate() error {
	if co.namespace == "" {
		return fmt.Errorf("namespace unset")
	}
	if co.name == "" {
		return fmt.Errorf("name unset")
	}
	if co.source == "" {
		return fmt.Errorf("source unset")
	}
	return nil
}

func (args *flagArray) String() string {
	return strings.Join(args.toStringArr(), ", ")
}

func (args *flagArray) Set(value string) error {
	*args = append(*args, value)
	return nil
}

func (args *flagArray) toStringArr() []string {
	return []string(*args)
}
