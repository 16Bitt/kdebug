package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"time"

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
)

type clientWrapper struct {
	api    *kubernetes.Clientset
	config *rest.Config
	ctx    context.Context
}

type kubeResourceType int

const (
	kubeResourceTypeUnknown     = kubeResourceType(0)
	kubeResourceTypeDeployment  = kubeResourceType(1)
	kubeResourceTypeJob         = kubeResourceType(2)
	kubeResourceTypeCronJob     = kubeResourceType(3)
	kubeResourceTypeStatefulSet = kubeResourceType(4)
)

type flagArray []string

type cliOptions struct {
	namespace     string
	name          string
	rawSourceType string
	source        string
	containerName string
	image         string
	entrypoint    []string
	timeout       time.Duration
}

type dimQueue chan remotecommand.TerminalSize

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

	log.Printf("Fetching %s/%s in namespace %s...", options.rawSourceType, options.source, options.namespace)
	switch options.resourceType() {
	case kubeResourceTypeDeployment:
		podSpec, err = client.getDeploymentPod(options.namespace, options.source)
	case kubeResourceTypeJob:
		podSpec, err = client.getJobPod(options.namespace, options.source)
	case kubeResourceTypeCronJob:
		podSpec, err = client.getCronJobPod(options.namespace, options.source)
	case kubeResourceTypeStatefulSet:
		podSpec, err = client.getStatefulSetPod(options.namespace, options.source)
	default:
		panic("unhandled resource type")
	}

	if err != nil {
		panic(err)
	}
	log.Printf("Generating spec...")
	pod, err := options.podFromSpec(podSpec)
	if err != nil {
		panic(err)
	}
	log.Printf("Creating pod %s in namespace %s...", options.name, options.namespace)
	created, err := client.schedule(pod)
	if err != nil {
		panic(err)
	}
	defer func() {
		log.Printf("Removing pod %s...", created.ObjectMeta.Name)
		err := client.terminate(created)
		if err != nil {
			log.Printf("Could not remove pod: %s", err)
		} else {
			log.Printf("Cleaned up successfully.")
		}
	}()

	log.Printf("Waiting for pod to start...")
	err = client.waitForPod(created)
	if err != nil {
		panic(err)
	}

	targetContainer := options.containerName
	if targetContainer == "" {
		targetContainer = created.Spec.Containers[0].Name
	}

	log.Printf("Spawning shell...")
	err = client.execAttached(created, targetContainer, []string{"/bin/sh"})
	if err != nil {
		panic(err)
	}
}

func newCliOptions() *cliOptions {
	entryArgs := &flagArray{}

	ns := flag.String("namespace", "default", "Namespace for resource and debug pod")
	name := flag.String("name", "kdebug-pod", "Name of debugging pod created")
	sourceType := flag.String("type", "deployment", "Resource type to debug")
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
		rawSourceType: *sourceType,
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
	if co.resourceType() == kubeResourceTypeUnknown {
		return fmt.Errorf("invalid resource type")
	}
	if co.source == "" {
		return fmt.Errorf("source unset")
	}
	return nil
}

func (co *cliOptions) resourceType() kubeResourceType {
	return newKubeResourceType(co.rawSourceType)
}

func (co *cliOptions) podFromSpec(spec *corev1.PodSpec) (*corev1.Pod, error) {
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

	if len(co.entrypoint) == 0 {
		container.Command = []string{"/bin/sleep", fmt.Sprintf("%.0f", co.timeout.Seconds())}
	} else {
		container.Command = co.entrypoint
	}
	container.Args = []string{}
	container.StartupProbe = nil
	container.ReadinessProbe = nil
	container.LivenessProbe = nil

	return pod, nil
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
			log.Printf("Pod status is now '%s'", updated.Status.Phase)
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

func newKubeResourceType(t string) kubeResourceType {
	switch t {
	case "deployment":
		return kubeResourceTypeDeployment
	case "job":
		return kubeResourceTypeJob
	case "cronjob":
		return kubeResourceTypeCronJob
	case "statefulset":
		return kubeResourceTypeStatefulSet
	default:
		return kubeResourceTypeUnknown
	}
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

// monitor spawns a goroutine to poll the current terminal dimensions and
// enqueues them into a dimQueue. A chan is returned that can be used to signal
// the shutdown of this goroutine.
func (q dimQueue) monitor() chan bool {
	cancel := make(chan bool)

	go func() {
		for {
			select {
			case <-cancel:
				log.Printf("Stopping resize monitor")
				close(cancel)
				return
			case <-time.After(5 * time.Second):
				q.update()
			}
		}
	}()

	return cancel
}

func (q dimQueue) Next() *remotecommand.TerminalSize {
	newDim, ok := <-q
	if !ok {
		return nil
	}

	return &newDim
}

func (q dimQueue) update() {
	width, height, err := term.GetSize(0)
	if err != nil {
		panic(err)
	}
	q <- remotecommand.TerminalSize{Width: uint16(width), Height: uint16(height)}
}
