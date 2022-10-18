package main

import (
	"log"

	corev1 "k8s.io/api/core/v1"
)

func main() {
	var podSpec *corev1.PodSpec
	var err error

	options := newCliOptions()
	if err := options.validate(); err != nil {
		panic(err)
	}
	client, err := newClientWrapper()
	if err != nil {
		panic(err)
	}

	switch options.resourceType {
	case "deployment":
		podSpec, err = client.getDeploymentPod(options.namespace, options.source)
	case "job":
		podSpec, err = client.getJobPod(options.namespace, options.source)
	case "cronjob":
		podSpec, err = client.getCronJobPod(options.namespace, options.source)
	case "statefulset":
		podSpec, err = client.getStatefulSetPod(options.namespace, options.source)
	default:
		log.Fatalf("unhandled resource type %s", options.resourceType)
	}

	if err != nil {
		panic(err)
	}
	pod, err := podFromSpec(options, podSpec)
	if err != nil {
		panic(err)
	}
	created, err := client.schedule(pod)
	if err != nil {
		panic(err)
	}
	defer func() {
		err := client.terminate(created)
		if err != nil {
			log.Printf("Could not remove pod: %s", err)
		}
	}()

	err = client.waitForPod(created)
	if err != nil {
		panic(err)
	}

	targetContainer := options.containerName
	if targetContainer == "" {
		targetContainer = created.Spec.Containers[0].Name
	}

	err = client.execAttached(created, targetContainer, []string{"/bin/sh"})
	if err != nil {
		panic(err)
	}
}
