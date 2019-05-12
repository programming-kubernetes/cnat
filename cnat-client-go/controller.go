package main

import (
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
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

const (
	// SuccessSynced is used as part of the Event 'reason' when a At is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a At fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by At"
	// MessageResourceSynced is the message used for an Event fired when a At
	// is synced successfully
	MessageResourceSynced = "At synced successfully"
)

// Controller is the controller implementation for At resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeClientset kubernetes.Interface
	// cnatClientset is a clientset for our own API group
	cnatClientset clientset.Interface
	atLister      listers.AtLister
	atsSynced     cache.InformerSynced

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
	atInformer informers.AtInformer) *Controller {

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
		if err := c.syncHandler(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
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
// with the current status of the resource.
func (c *Controller) syncHandler(key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the At resource with this namespace/name
	at, err := c.atLister.Ats(namespace).Get(name)
	if err != nil {
		// The At resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("at '%s' in work queue no longer exists", key))
			return nil
		}

		return err
	}

	// deploymentName := at.Spec.DeploymentName
	// if deploymentName == "" {
	// 	// We choose to absorb the error here as the worker would requeue the
	// 	// resource otherwise. Instead, the next time the resource is updated
	// 	// the resource will be queued again.
	// 	utilruntime.HandleError(fmt.Errorf("%s: deployment name must be specified", key))
	// 	return nil
	// }

	// Get the deployment with the name specified in At.spec
	// deployment, err := c.deploymentsLister.Deployments(at.Namespace).Get(deploymentName)
	// If the resource doesn't exist, we'll create it
	// if errors.IsNotFound(err) {
	// 	deployment, err = c.kubeClientset.AppsV1().Deployments(at.Namespace).Create(newDeployment(at))
	// }

	// If an error occurs during Get/Create, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil {
		return err
	}

	// If the Deployment is not controlled by this At resource, we should log
	// a warning to the event recorder and ret
	// if !metav1.IsControlledBy(deployment, at) {
	// 	msg := fmt.Sprintf(MessageResourceExists, deployment.Name)
	// 	c.recorder.Event(at, corev1.EventTypeWarning, ErrResourceExists, msg)
	// 	return fmt.Errorf(msg)
	// }

	// If this number of the replicas on the At resource is specified, and the
	// number does not equal the current desired replicas on the Deployment, we
	// should update the Deployment resource.
	// if at.Spec.Replicas != nil && *at.Spec.Replicas != *deployment.Spec.Replicas {
	// 	klog.V(4).Infof("At %s replicas: %d, deployment replicas: %d", name, *at.Spec.Replicas, *deployment.Spec.Replicas)
	// 	deployment, err = c.kubeClientset.AppsV1().Deployments(at.Namespace).Update(newDeployment(at))
	// }

	// If an error occurs during Update, we'll requeue the item so we can
	// attempt processing again later. THis could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil {
		return err
	}

	// Finally, we update the status block of the At resource to reflect the
	// current state of the world
	err = c.updateAtStatus(at)
	if err != nil {
		return err
	}

	c.recorder.Event(at, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func (c *Controller) updateAtStatus(foo *cnatv1alpha1.At) error { //, deployment *appsv1.Deployment) error {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	atCopy := foo.DeepCopy()
	// atCopy.Status.AvailableReplicas = deployment.Status.AvailableReplicas
	// If the CustomResourceSubresources feature gate is not enabled,
	// we must use Update instead of UpdateStatus to update the Status block of the At resource.
	// UpdateStatus will not allow changes to the Spec of the resource,
	// which is ideal for ensuring nothing other than resource status has been updated.
	_, err := c.cnatClientset.CnatV1alpha1().Ats(foo.Namespace).Update(atCopy)
	return err
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

// handleObject will take any resource implementing metav1.Object and attempt
// to find the At resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that At resource to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
func (c *Controller) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		klog.V(4).Infof("Recovered deleted object '%s' from tombstone", object.GetName())
	}
	klog.V(4).Infof("Processing object: %s", object.GetName())
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If this object is not owned by a At, we should not do anything more
		// with it.
		if ownerRef.Kind != "At" {
			return
		}

		foo, err := c.atLister.Ats(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			klog.V(4).Infof("ignoring orphaned object '%s' of foo '%s'", object.GetSelfLink(), ownerRef.Name)
			return
		}

		c.enqueueAt(foo)
		return
	}
}

// newDeployment creates a new Deployment for a At resource. It also sets
// the appropriate OwnerReferences on the resource so handleObject can discover
// the At resource that 'owns' it.
func newDeployment(foo *cnatv1alpha1.At) *appsv1.Deployment {
	labels := map[string]string{
		"app":        "nginx",
		"controller": foo.Name,
	}
	var repl int32
	repl = 1
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test", //foo.Spec.DeploymentName,
			Namespace: foo.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(foo, schema.GroupVersionKind{
					Group:   cnatv1alpha1.SchemeGroupVersion.Group,
					Version: cnatv1alpha1.SchemeGroupVersion.Version,
					Kind:    "At",
				}),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &repl, //foo.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:latest",
						},
					},
				},
			},
		},
	}
}
