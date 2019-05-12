package main

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	corev1informer "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	cnatv1alpha1 "github.com/programming-kubernetes/cnat/cnat-client-go/pkg/apis/cnat/v1alpha1"
	clientset "github.com/programming-kubernetes/cnat/cnat-client-go/pkg/generated/clientset/versioned"
	cnatscheme "github.com/programming-kubernetes/cnat/cnat-client-go/pkg/generated/clientset/versioned/scheme"
	informers "github.com/programming-kubernetes/cnat/cnat-client-go/pkg/generated/informers/externalversions/cnat/v1alpha1"
	listers "github.com/programming-kubernetes/cnat/cnat-client-go/pkg/generated/listers/cnat/v1alpha1"
)

const controllerAgentName = "cnat-controller"

// Controller is the controller implementation for At resources
type Controller struct {
	kubeClientset kubernetes.Interface
	cnatClientset clientset.Interface

	atLister  listers.AtLister
	atsSynced cache.InformerSynced

	podLister  corev1lister.PodLister
	podsSynced cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new cnat controller
func NewController(
	kubeClientset kubernetes.Interface,
	cnatClientset clientset.Interface,
	atInformer informers.AtInformer,
	podInformer corev1informer.PodInformer) *Controller {

	// Create event broadcaster
	// Add cnat-controller types to the default Kubernetes Scheme so Events can be
	// logged for cnat-controller types.
	utilruntime.Must(cnatscheme.AddToScheme(scheme.Scheme))
	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeClientset: kubeClientset,
		cnatClientset: cnatClientset,
		atLister:      atInformer.Lister(),
		atsSynced:     atInformer.Informer().HasSynced,
		podLister:     podInformer.Lister(),
		podsSynced:    podInformer.Informer().HasSynced,
		workqueue:     workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Ats"),
		recorder:      recorder,
	}

	klog.Info("Setting up event handlers")
	// Set up an event handler for when At resources change
	atInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueAt,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueAt(new)
		},
	})
	// Set up an event handler for when Pod resources change
	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueuePod,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueuePod(new)
		},
	})
	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	klog.Info("Starting cnat client-go controller")

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.atsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	if ok := cache.WaitForCacheSync(stopCh, c.podsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	// Launch two workers to process At resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// At resource to be synced.
		if when, err := c.syncHandler(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		} else if when != time.Duration(0) {
			c.workqueue.AddAfter(key, when)
		} else {
			// Finally, if no error occurs we Forget this item so it does not
			// get queued again until another change happens.
			c.workqueue.Forget(obj)
		}
		klog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the At resource
// with the current status of the resource. It returns how long to wait
// until the schedule is due.
func (c *Controller) syncHandler(key string) (time.Duration, error) {
	klog.Infof("=== Reconciling At %s", key)

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return time.Duration(0), nil
	}

	// Get the At resource with this namespace/name
	instance, err := c.atLister.Ats(namespace).Get(name)
	if err != nil {
		// The At resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("at '%s' in work queue no longer exists", key))
			return time.Duration(0), nil
		}

		return time.Duration(0), err
	}

	// If no phase set, default to pending (the initial phase):
	if instance.Status.Phase == "" {
		instance.Status.Phase = cnatv1alpha1.PhasePending
	}

	// Now let's make the main case distinction: implementing
	// the state diagram PENDING -> RUNNING -> DONE
	switch instance.Status.Phase {
	case cnatv1alpha1.PhasePending:
		klog.Infof("instance %s: phase=PENDING", key)
		// As long as we haven't executed the command yet,  we need to check if it's time already to act:
		klog.Infof("instance %s: checking schedule %q", key, instance.Spec.Schedule)
		// Check if it's already time to execute the command with a tolerance of 2 seconds:
		d, err := timeUntilSchedule(instance.Spec.Schedule)
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("schedule parsing failed: %v", err))
			// Error reading the schedule - requeue the request:
			return time.Duration(0), err
		}
		klog.Infof("instance %s: schedule parsing done: diff=%v", key, d)
		if d > 0 {
			// Not yet time to execute the command, wait until the scheduled time
			return d, nil
		}

		klog.Infof("instance %s: it's time! Ready to execute: %s", key, instance.Spec.Command)
		instance.Status.Phase = cnatv1alpha1.PhaseRunning
	case cnatv1alpha1.PhaseRunning:
		klog.Infof("instance %s: Phase: RUNNING", key)

		pod := newPodForCR(instance)

		// Set At instance as the owner and controller
		owner := metav1.NewControllerRef(instance, cnatv1alpha1.SchemeGroupVersion.WithKind("At"))
		pod.ObjectMeta.OwnerReferences = append(pod.ObjectMeta.OwnerReferences, *owner)

		// Try to see if the pod already exists and if not
		// (which we expect) then create a one-shot pod as per spec:
		found, err := c.kubeClientset.CoreV1().Pods(pod.Namespace).Get(pod.Name, metav1.GetOptions{})
		if err != nil && errors.IsNotFound(err) {
			found, err = c.kubeClientset.CoreV1().Pods(pod.Namespace).Create(pod)
			if err != nil {
				return time.Duration(0), err
			}
			klog.Infof("instance %s: pod launched: name=%s", key, pod.Name)
		} else if err != nil {
			// requeue with error
			return time.Duration(0), err
		} else if found.Status.Phase == corev1.PodFailed || found.Status.Phase == corev1.PodSucceeded {
			klog.Infof("instance %s: container terminated: reason=%q message=%q", key, found.Status.Reason, found.Status.Message)
			instance.Status.Phase = cnatv1alpha1.PhaseDone
		} else {
			// don't requeue because it will happen automatically when the pod status changes
			return time.Duration(0), nil
		}
	case cnatv1alpha1.PhaseDone:
		klog.Infof("instance %s: phase: DONE", key)
		return time.Duration(0), nil
	default:
		klog.Infof("instance %s: NOP")
		return time.Duration(0), nil
	}

	// Update the At instance, setting the status to the respective phase:
	_, err = c.cnatClientset.CnatV1alpha1().Ats(instance.Namespace).UpdateStatus(instance)
	if err != nil {
		return time.Duration(0), err
	}

	// Don't requeue. We should be reconcile because either the pod or the CR changes.
	return time.Duration(0), nil
}

// enqueueAt takes a At resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than At.
func (c *Controller) enqueueAt(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

// enqueueAt a pod and checks that the owner reference points to an At object. It then
// enqueues this At object.
func (c *Controller) enqueuePod(obj interface{}) {
	var pod *corev1.Pod
	var ok bool
	if pod, ok = obj.(*corev1.Pod); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding pod, invalid type"))
			return
		}
		pod, ok = tombstone.Obj.(*corev1.Pod)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding pod tombstone, invalid type"))
			return
		}
		klog.V(4).Infof("Recovered deleted pod '%s' from tombstone", pod.GetName())
	}
	if ownerRef := metav1.GetControllerOf(pod); ownerRef != nil {
		if ownerRef.Kind != "At" {
			return
		}

		at, err := c.atLister.Ats(pod.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			klog.V(4).Infof("ignoring orphaned pod '%s' of At '%s'", pod.GetSelfLink(), ownerRef.Name)
			return
		}

		klog.Infof("enqueuing At %s/%s because pod changed", at.Namespace, at.Name)
		c.enqueueAt(at)
	}
}

// newPodForCR returns a busybox pod with the same name/namespace as the cr
func newPodForCR(cr *cnatv1alpha1.At) *corev1.Pod {
	labels := map[string]string{
		"app": cr.Name,
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-pod",
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "busybox",
					Image:   "busybox",
					Command: strings.Split(cr.Spec.Command, " "),
				},
			},
			RestartPolicy: corev1.RestartPolicyOnFailure,
		},
	}
}

// timeUntilSchedule parses the schedule string and returns the time until the schedule.
// When it is overdue, the duration is negative.
func timeUntilSchedule(schedule string) (time.Duration, error) {
	now := time.Now().UTC()
	layout := "2006-01-02T15:04:05Z"
	s, err := time.Parse(layout, schedule)
	if err != nil {
		return time.Duration(0), err
	}
	return s.Sub(now), nil
}
