package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"

	"golang.org/x/term"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/homedir"

	_ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	_ "k8s.io/client-go/plugin/pkg/client/auth/exec"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

type clientWrapper struct {
	api    *kubernetes.Clientset
	config *rest.Config
	ctx    context.Context
}

func newClientWrapper() (*clientWrapper, error) {
	cfgPath := path.Join(homedir.HomeDir(), ".kube", "config")
	cfg, err := clientcmd.BuildConfigFromFlags("", cfgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	k8s, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build client: %w", err)
	}
	return &clientWrapper{
		api:    k8s,
		config: cfg,
		ctx:    context.TODO(),
	}, nil
}

func (c *clientWrapper) getDeploymentPod(namespace, name string) (*corev1.PodSpec, error) {
	dep, err := c.api.AppsV1().Deployments(namespace).Get(c.ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return (&dep.Spec.Template.Spec).DeepCopy(), nil
}

func (c *clientWrapper) getJobPod(namespace, name string) (*corev1.PodSpec, error) {
	job, err := c.api.BatchV1().Jobs(namespace).Get(c.ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return (&job.Spec.Template.Spec).DeepCopy(), nil
}

func (c *clientWrapper) getCronJobPod(namespace, name string) (*corev1.PodSpec, error) {
	cronjob, err := c.api.BatchV1().CronJobs(namespace).Get(c.ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return (&cronjob.Spec.JobTemplate.Spec.Template.Spec).DeepCopy(), nil
}

func (c *clientWrapper) getStatefulSetPod(namespace, name string) (*corev1.PodSpec, error) {
	statefulset, err := c.api.AppsV1().StatefulSets(namespace).Get(c.ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return (&statefulset.Spec.Template.Spec).DeepCopy(), nil
}

func (c *clientWrapper) schedule(pod *corev1.Pod) (*corev1.Pod, error) {
	return c.api.CoreV1().Pods(pod.ObjectMeta.Namespace).Create(c.ctx, pod, metav1.CreateOptions{})
}

func (c *clientWrapper) terminate(pod *corev1.Pod) error {
	return c.api.CoreV1().Pods(pod.ObjectMeta.Namespace).Delete(c.ctx, pod.ObjectMeta.Name, metav1.DeleteOptions{})
}

func (c *clientWrapper) waitForPod(pod *corev1.Pod) error {
	watcher, err := c.api.CoreV1().Pods(pod.ObjectMeta.Namespace).Watch(c.ctx, metav1.SingleObject(pod.ObjectMeta))
	if err != nil {
		return err
	}
	defer watcher.Stop()

	for {
		evt := <-watcher.ResultChan()
		switch evt.Type {
		case watch.Modified:
			updated := evt.Object.(*corev1.Pod)
			if updated.Status.Phase == corev1.PodFailed {
				return fmt.Errorf("pod failed to start")
			}
			if updated.Status.Phase == corev1.PodRunning {
				return nil
			}
		default:
			return fmt.Errorf("unexpected event %+v", evt.Type)
		}
	}
}

func (c *clientWrapper) execAttached(pod *corev1.Pod, containerName string, command []string) error {
	req := c.api.CoreV1().
		RESTClient().
		Post().
		Namespace(pod.ObjectMeta.Namespace).
		Resource("pods").
		Name(pod.ObjectMeta.Name).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   command,
			Stdout:    true,
			Stdin:     true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)

	process, err := remotecommand.NewSPDYExecutor(c.config, "POST", req.URL())
	if err != nil {
		return err
	}

	oldTerm, err := term.MakeRaw(0)
	if err != nil {
		return err
	}
	defer func() {
		err := term.Restore(0, oldTerm)
		if err != nil {
			log.Printf("Failed to restore terminal settings. Use the reset command to fix. Error was %s", err)
		}
	}()

	queue := make(dimQueue, 2)
	queue.update()
	cancel := queue.monitor()
	defer func() {
		close(queue)
		cancel <- true
	}()

	return process.Stream(remotecommand.StreamOptions{
		Stdin:             os.Stdin,
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
		Tty:               true,
		TerminalSizeQueue: queue,
	})
}
