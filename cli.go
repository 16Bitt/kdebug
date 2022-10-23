package main

import (
	"flag"
	"fmt"
	"strings"
)

type flagArray struct {
	// pristine indicates if the flagArray still contains default values. The
	// first call to Set when pristine is true will clear pristine and the
	// underlying elements.
	pristine bool
	elements []string
}

type cliOptions struct {
	namespace     string
	name          string
	resourceType  string
	source        string
	containerName string
	image         string
	entrypoint    []string
	command       []string
}

func newCliOptions() *cliOptions {
	entryArgs := &flagArray{
		pristine: true,
		elements: []string{"/bin/sleep", "1800"},
	}
	commandArgs := &flagArray{
		pristine: true,
		elements: []string{"/bin/sh"},
	}

	ns := flag.String("namespace", "default", "Namespace for resource and debug pod")
	name := flag.String("name", "kdebug-pod", "Name of debugging pod created")
	resourceType := flag.String("type", "deployment", "Resource type to debug")
	source := flag.String("source", "", "Resource name to debug")
	containerName := flag.String("container-name", "", "Container name to target, if set, otherwise uses the first container")
	image := flag.String("image", "", "Image to use, if set")
	flag.Var(entryArgs, "entry", "Container entrypoint. Repeat this flag to pass multiple arguments.")
	flag.Var(commandArgs, "command", "Command to execute in an interactive shell. Repeat this flag to pass multiple arguments.")
	flag.Parse()

	return &cliOptions{
		namespace:     *ns,
		name:          *name,
		resourceType:  *resourceType,
		source:        *source,
		entrypoint:    entryArgs.elements,
		command:       commandArgs.elements,
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
	return strings.Join(args.elements, " ")
}

func (args *flagArray) Set(value string) error {
	if args.pristine {
		args.pristine = false
		args.elements = []string{}
	}

	args.elements = append(args.elements, value)
	return nil
}
