package main

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

func podFromSpec(co *cliOptions, spec *corev1.PodSpec) (*corev1.Pod, error) {
	pod := &corev1.Pod{}
	pod.ObjectMeta.Name = co.name
	pod.ObjectMeta.Namespace = co.namespace

	// TODO: probably shouldn't mutate a referenced object -- deepcopy beforehand?
	pod.Spec = *spec
	pod.Spec.RestartPolicy = corev1.RestartPolicyNever
	*pod.Spec.TerminationGracePeriodSeconds = int64(0)

	container := &spec.Containers[0]
	if co.containerName != "" {
		found := false
		for _, c := range spec.Containers {
			if c.Name == co.containerName {
				found = true
				container = &c
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("could not find container '%s' within pod spec", co.containerName)
		}
	}

	container.Command = co.entrypoint
	container.Args = []string{}
	container.StartupProbe = nil
	container.ReadinessProbe = nil
	container.LivenessProbe = nil

	return pod, nil
}
