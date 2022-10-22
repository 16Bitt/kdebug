# kdebug

A small tool to make debugging workloads in Kubernetes easier.

# What does this solve?

## Problem

`kdebug` addresses a frustration I've had many times with Kubernetes: I frequently need to debug
workloads that are either not running (crashed) or do not run perpetually. There are several
options, but they are largely sub-optimal:

1. Manually copy the pod template from the desired resource, trim out all of the noise, and apply it
   to the cluster.
2. Use the new-ish feature `kubectl debug`. This is fine _but_ it does not help in the case of pods
   that have never run (e.g. a cronjob), and it is definitely not streamlined for the use-case of
   "clone a pod and give me a shell session".
3. `kubectl exec` into a running pod. For workloads with really dialed-in resources, you'll likely
   OOMKill under any sort of load by doing this. Regardless, this is the easiest way if the pod is
   still running, and it's how I see most folks handle this problem.
4. Use `kubectl create` and manually specify 50 (exaggeration) flags to recreate the pod context you
   need.
5. Use `kubectl edit` and mess up a production workload for debugging.
6. Add a constantly running deployment for engineers to `exec` in, which is going to consume compute
   resources (and by extension, money) for something that is not used 24/7.

I think a lot of the available tools miss the most critical part of _why_ a developer needs to debug
in a cluster -- the context. All of it. The volumes, the secrets, and the configmaps. If I didn't
need this context, I would run the application locally. In particular, engineers working with
frameworks like Rails frequently need to open a REPL within a kubernetes cluster. This uses a
non-negligible amount of memory, and you really don't want debugging to interfere with your
production workloads.

Bizarrely enough, `kubectl create job --from cronjob/cronjob-name` is the closest to what I want: it
spawns a job using the template of another resource. Unfortunately the options are very restrictive,
and it appears to be the only sub-resource that can be spawned directly from a parent through
`kubectl create`.

## How does kdebug solve this?

`kdebug` automates option 1:

1. The desired resource is fetched
2. The pod spec is extracted from the resource
3. Various parts of the pod spec are overwritten or removed (command, ports, liveness, etc)
4. The pod is spawned in the cluster
5. An interactive shell is attached
6. The pod is cleaned up when the shell terminates

This could easily be a shell script, but I figured that using the `client-go` library for kubernetes
would be a bit more stable and easier to distribute.

# Installation

## Prerequisites

- `go` (should build with any `go` version supporting Go Modules)
- `make` is recommended, but optional (tested with GNU make v4.3, but any `make` should work)

## Build and Install

The default will cross compile for different architectures:

```sh
make
```

| OS             | Architecture                | Executable                 |
|----------------|-----------------------------|----------------------------|
| Linux          | ARM64                       | build/kdebug-linux-arm64   |
| Linux          | AMD64 (aka intel or x86-64) | build/kdebug-linux-x86_64  |
| MacOS (darwin) | M1/M2 (ARM64)               | build/kdebug-darwin-arm64  |
| MacOS (darwin) | AMD64 (aka intel or x86-64) | build/kdebug-darwin-x64_64 |
| Host           | Host                        | build/kdebug-noarch        |

Build just for your machine:

```sh
make build/kdebug-noarch
```

Once the build is complete, copy the appropriate executable from `build` to your `$PATH`.

# Usage

**WARNING: `kdebug` uses the default go command line argument parsing _not_ the GNU-style parsing
that kubectl uses. This will not behave anything like `kubectl`.**

By default, the entrypoint of the pod will be set to `/bin/sleep 1800` to terminate the pod after 30
minutes. This is intended to prevent unused pods littering your namespaces.

```
Usage of kdebug:
  -container-name string
    	Container name to target, if set, otherwise uses the first container
  -entry value
    	Entrypoint executable to execute while connecting a shell (repeat the flag to pass arguments)
  -image string
    	Image to use, if set
  -name string
    	Name of debugging pod created (default "kdebug-pod")
  -namespace string
    	Namespace for resource and debug pod (default "default")
  -source string
    	Resource name to debug
  -timeout string
    	Timeout for the entrypoint. Only used if entrypoint is not overridden. (default "30m")
  -type string
    	Resource type to debug (default "deployment")
```

## Spawn a debug pod with the default settings

```sh
kdebug -namespace rails-demo -type deployment -source rails-demo-deployment
```

## Spawn a debug pod with custom entrypoint arguments

```sh
kdebug -namespace rails-demo \
  -type deployment \
  -source rails-demo-deployment \
  -entry /bin/sh -entry "-c" -entry "while true; do sleep 10; done"
```

## Spawn a debug pod with a long timeout

```sh
kdebug -namespace rails-demo \
  -type deployment \
  -source rails-demo-deployment \
  -timeout 24h
```

# Bugs and sharp edges

- The `flag` parsing is pretty weird if you're used to `kubectl`. Perhaps something like `kdebug
  deployment my-deployment -namespace foo-bar` would be a bit more intuitive.
- No option for specifying the entrypoint command (`/bin/sh` is hard-coded)
- No support for containers that don't have any interactive environment (e.g. `FROM scratch` images)
- No support overwriting the resource configuration
- The way the `kubecfg` is loaded does not load the default namespace for the context. This forces
  the user to specify the namespace anyway.
