package at

import (
	"context"
	"fmt"
	"strings"
	"time"

	cnatv1alpha1 "github.com/programming-kubernetes/cnat/cnat-operator/pkg/apis/cnat/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_at")

// Add creates a new At Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileAt{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("at-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource At
	err = c.Watch(&source.Kind{Type: &cnatv1alpha1.At{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner At
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &cnatv1alpha1.At{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileAt{}

// ReconcileAt reconciles a At object
type ReconcileAt struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a At object and makes changes based on the state read
// and what is in the At.Spec
func (r *ReconcileAt) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("namespace", request.Namespace, "at", request.Name)
	reqLogger.Info("=== Reconciling At")
	// Fetch the At instance
	instance := &cnatv1alpha1.At{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request - return and don't requeue:
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request:
		return reconcile.Result{}, err
	}

	// If no phase set, default to pending (the initial phase):
	if instance.Status.Phase == "" {
		instance.Status.Phase = cnatv1alpha1.PhasePending
	}

	// Now let's make the main case distinction: implementing
	// the state diagram PENDING -> RUNNING -> DONE
	switch instance.Status.Phase {
	case cnatv1alpha1.PhasePending:
		reqLogger.Info("Phase: PENDING")
		// As long as we haven't executed the command yet,
		// we need to check if it's time already to act:
		reqLogger.Info("Checking schedule", "Target", instance.Spec.Schedule)
		// Check if it's already time to execute the command with a tolerance of 2 seconds:
		timetolaunch, cmresult, err := ready2Launch(instance.Spec.Schedule, 2*time.Second)
		if err != nil {
			reqLogger.Error(err, "Schedule parsing failure")
			// Error reading the schedule - requeue the request:
			return reconcile.Result{}, err
		}
		reqLogger.Info("Schedule parsing done", "Result", cmresult)
		if timetolaunch {
			reqLogger.Info("It's time!", "Ready to execute", instance.Spec.Command)
			instance.Status.Phase = cnatv1alpha1.PhaseRunning
		} else {
			// Not yet time to execute the command - requeue to try again in 10 seconds:
			return reconcile.Result{RequeueAfter: time.Second * 10}, nil
		}
	case cnatv1alpha1.PhaseRunning:
		reqLogger.Info("Phase: RUNNING")
		pod := newPodForCR(instance)
		// Set At instance as the owner and controller
		if err := controllerutil.SetControllerReference(instance, pod, r.scheme); err != nil {
			return reconcile.Result{}, err
		}
		found := &corev1.Pod{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, found)
		// Try to see if the pod already exists and if not
		// (which we expect) then create a one-shot pod as per spec:
		if err != nil && errors.IsNotFound(err) {
			err = r.client.Create(context.TODO(), pod)
			if err != nil {
				return reconcile.Result{}, err
			}
			reqLogger.Info("Pod launched", "name", pod.Name)
			instance.Status.Phase = cnatv1alpha1.PhaseDone
		}
	case cnatv1alpha1.PhaseDone:
		reqLogger.Info("Phase: DONE")
		pod := newPodForCR(instance)
		found := &corev1.Pod{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, found)
		if err != nil {
			reqLogger.Error(err, "Finding pod failure")
		}
		if found.Status.Phase == corev1.PodRunning {
			// reqLogger.Info("Pod running", "name", found.Name, "Pod.Phase", found.Status.Phase)
			maincontainerstate := found.Status.ContainerStatuses[0].State
			if maincontainerstate.Terminated != nil {
				reqLogger.Info("Main container terminated", "Reason", maincontainerstate.Terminated.Reason)
			}
		}
	default:
	}

	// Update the At instance, setting the status to the respective phase:
	err = r.client.Update(context.TODO(), instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Not yet time to execute the command - requeue to try again in 10 seconds:
	return reconcile.Result{RequeueAfter: time.Second * 10}, nil
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

// ready2Launch return true IFF the schedule is within tolerance, for example:
// if the schedule is 2019-04-11T10:10:00Z and it's now 2019-04-11T10:10:02Z
// and the tolerance provided is 5 seconds, then this function returns true.
func ready2Launch(schedule string, tolerance time.Duration) (bool, string, error) {
	now := time.Now().UTC()
	layout := "2006-01-02T15:04:05Z"
	s, err := time.Parse(layout, schedule)
	if err != nil {
		return false, "", err
	}
	diff := s.Sub(now)
	cmpresult := fmt.Sprintf("%v with a diff of %v to %v", s, diff, now)
	if time.Until(s) < time.Duration(tolerance) {
		return true, cmpresult, nil
	}
	return false, cmpresult, nil
}
