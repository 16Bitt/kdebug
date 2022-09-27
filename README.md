# kdebug

A small tool to make debugging workloads in Kubernetes easier.

# What does this solve?

## Problem

`kdebug` addresses a frustration I've had many times with Kubernetes: I frequently need to debug
workloads that are either not running (crashed) or do not run perpetually. There are only a few
options:

1. Manually copy the pod template from the desired resource, trim out all of the noise, and apply it
   to the cluster.
2. Use the new-ish feature `kubectl debug`. This is fine _but_ it does not help in the case of pods
   that have never run (e.g. a cronjob).
3. `kubectl exec` into a running pod. For workloads with really dialed-in resources, you'll likely
   OOM kill a pod running a real workload by doing this. Regardless, this is the easiest way if the
   pod is still running.
4. Use `kubectl create` and manually specify 50 (exaggeration) flags to recreate the pod context you
   need.
5. Use `kubectl edit` and mess up a production workload for debugging.

I think a lot of the available tools miss the most critical part of _why_ a developer needs to debug
in a cluster -- the context. All of it. The volumes, the secrets, and the configmaps. If I didn't
need this context, I would run the application locally.

Bizarrely enough, `kubectl create job --from cronjob/cronjob-name` is the closest to what I want: it
spawns a job using the template of another resource. Unfortunately the options are very restrictive.

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

# Usage

**WARNING: `kdebug` uses the default go command line argument parsing _not_ the GNU-style parsing
that kubectl uses. This will not behave anything like `kubectl`.**

By default, the entrypoint of the pod will be set to `/bin/sleep 600` to terminate the pod after 10
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

# Bugs and sharp edges

- The `flag` parsing is pretty weird if you're used to `kubectl`. Perhaps something like `kdebug
  deployment my-deployment -namespace foo-bar` would be a bit more intuitive.
- No option for specifying the entrypoint command (`/bin/sh` is hard-coded)
- No support for containers that don't have any interactive environment (e.g. `FROM scratch` images)
- No support overwriting the resource configuration
- The output is very noisy
