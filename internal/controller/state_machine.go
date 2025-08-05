package controller

import (
	"context"
	"fmt"
	"time"

	api "github.com/kanisterio/datamover/api/v1alpha1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type State int

const (
	None State = iota
	Init

	ValidationFailed
	CreateResourcesSuccess
	// TODO: these states are reachable only if we introduce a retry limit
	// CreateResourcesFailedDirty
	// CreateResourcesFailedClean
	CreateResourcesInProgress

	ReadinessWait
	ReadinessSuccess
	ReadinessResourcesMissing
	ReadinessResourcesFailure

	ReadinessFailedDirty
	ReadinessFailedClean

	SessionRunning
	SessionResourcesFailure
	SessionFailedDirty
	SessionFailedClean

	// These states are outside of reconcile loop
	// Empty
	// EmptyTerminating
)

func (r *DatamoverSessionReconciler) Run(ctx context.Context, dmSession *api.DatamoverSession) (ctrl.Result, error) {
	requeue_wait_sec := func(sec time.Duration) ctrl.Result {
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * sec}
	}

	state, resources, err := r.GetState(ctx, dmSession)
	// FIXME: emit events to notify of errors in GetState
	if err != nil {
		// FIXME: error will requeue. Do we want to give up at some point?
		log.Log.Error(err, "Cannot get state machine state")
		return ctrl.Result{}, err
	}

	log.Log.Info("Processing State ", "state", state)

	switch state {
	case Init:
		log.Log.Info("Validating session")
		err := validateSession(*dmSession)
		if err != nil {
			log.Log.Error(err, "Session validation failed")
			err := r.UpdateStatus(ctx, dmSession, api.ProgressValidationFailed)
			if err != nil {
				return ctrl.Result{}, err
			}
			// Shortcut for terminal state, do not requeue
			return ctrl.Result{}, nil
		}
		err = r.tryCreateResources(ctx, *dmSession, resources)
		if err != nil {
			return ctrl.Result{}, err
		}
		// Resource creation should trigger further reconcile
		// Requeue in case it doesn't happen
		return requeue_wait_sec(20), nil
	case ValidationFailed:
		return ctrl.Result{}, nil
	case CreateResourcesSuccess:
		err := r.UpdateStatus(ctx, dmSession, api.ProgressResourcesCreated)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	case CreateResourcesInProgress:
		err := r.tryCreateResources(ctx, *dmSession, resources)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil

	case ReadinessWait:
		log.Log.Info("Waiting for readiness")
		return requeue_wait_sec(20), nil

	case ReadinessResourcesMissing:
		err := r.UpdateStatus(ctx, dmSession, api.ProgressReadinessFailure)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil

	case ReadinessResourcesFailure:
		err := r.UpdateStatus(ctx, dmSession, api.ProgressReadinessFailure)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil

	case ReadinessFailedDirty:
		// We don't cleanup the failed pod (to trace errors)
		err := r.CleanupService(ctx, dmSession, resources)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil

	case ReadinessFailedClean:
		return ctrl.Result{}, nil

	case ReadinessSuccess:
		if resources == nil {
			return ctrl.Result{}, fmt.Errorf("Invalid state. Resources cannot be empty in ReadinessSuccess")
		}

		err := r.UpdateStatusData(ctx, dmSession, api.ProgressReady, *resources)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil

	case SessionRunning:
		return ctrl.Result{}, nil

	case SessionResourcesFailure:
		err := r.UpdateStatus(ctx, dmSession, api.ProgressSessionFailure)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	case SessionFailedDirty:
		// We don't cleanup the failed pod (to trace errors)
		err := r.CleanupService(ctx, dmSession, resources)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil

	case SessionFailedClean:
		return ctrl.Result{}, nil

	case None:
		return ctrl.Result{}, fmt.Errorf("Invalid state. Unknown state None. Should have returned an error from GetState")

	default:
		return ctrl.Result{}, fmt.Errorf("Invalid state. Unknown state %v", state)
	}
}

func (r *DatamoverSessionReconciler) tryCreateResources(ctx context.Context, dmSession api.DatamoverSession, resources *resources) error {
	err := r.CreateResources(ctx, dmSession, resources)

	if err != nil {
		// Don't fail if resource already exists
		// Read consistency is not too good here
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		log.Log.Error(err, "Failed to create resources")
		return err
	}
	return nil
}

func (r *DatamoverSessionReconciler) CreateResources(ctx context.Context, dmSession api.DatamoverSession, resources *resources) error {
	if resources.pod == nil {
		err := r.CreatePod(ctx, dmSession)
		if err != nil {
			return err
		}
	}
	if resources.service == nil && resources.needService {
		err := r.CreateService(ctx, dmSession)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *DatamoverSessionReconciler) CleanupService(ctx context.Context, dmSession *api.DatamoverSession, resources *resources) error {
	if resources == nil {
		return nil
	}
	if resources.service != nil {
		err := r.DeleteService(ctx, resources.service)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *DatamoverSessionReconciler) CleanupPod(ctx context.Context, dmSession *api.DatamoverSession, resources *resources) error {
	if resources == nil {
		return nil
	}
	if resources.pod != nil {
		err := r.DeletePod(ctx, resources.pod)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *DatamoverSessionReconciler) UpdateStatus(ctx context.Context, dmSession *api.DatamoverSession, status api.DatamoverSessionProgress) error {
	dmSession.Status.Progress = status
	if err := r.Status().Update(ctx, dmSession); err != nil {
		// TODO: wrap error
		return err
	}
	log.Log.Info("Updated status to", "status", status)
	return nil
}

func (r *DatamoverSessionReconciler) UpdateStatusData(ctx context.Context, dmSession *api.DatamoverSession, status api.DatamoverSessionProgress, resources resources) error {
	dmSession.Status.Progress = status
	if resources.podReadiness == nil {
		return fmt.Errorf("Invalid state. PodReadiness cannot be empty at this point")
	}
	serviceName := ""
	if resources.service != nil {
		serviceName = resources.service.Name
	}
	dmSession.Status.SessionInfo = api.SessionInfo{
		PodName:     resources.pod.Name,
		ServiceName: serviceName,
		SessionData: resources.podReadiness.data,
	}
	if err := r.Status().Update(ctx, dmSession); err != nil {
		// TODO: wrap error
		return err
	}
	return nil
}

func (r *DatamoverSessionReconciler) GetState(ctx context.Context, dmSession *api.DatamoverSession) (State, *resources, error) {
	resources, err := r.getResources(ctx, dmSession)
	if err != nil {
		return None, nil, errors.Wrap(err, "Error while getting resources")
	}

	switch dmSession.Status.Progress {
	case api.ProgressNone: // Progress is not set yet
		if dmSession.Status.SessionInfo.SessionData != "" {
			return None, nil, fmt.Errorf("Invalid state. Session data should be empty if progress not set")
		}
		// No resources
		if resourcesEmpty(*resources) {
			return Init, resources, nil
		}

		// All resources
		if resourcesExist(*resources) {
			return CreateResourcesSuccess, resources, nil
		}
		// Partial resources
		return CreateResourcesInProgress, resources, nil
	case api.ProgressValidationFailed:
		return ValidationFailed, nil, nil
	case api.ProgressResourcesCreated:
		// Resources missing
		if !resourcesExist(*resources) {
			return ReadinessResourcesMissing, resources, nil
		}

		if resourcesFailed(*resources) {
			return ReadinessResourcesFailure, resources, nil
		}

		if resourcesReady(*resources) {
			return ReadinessSuccess, resources, nil
		}
		return ReadinessWait, resources, nil
	case api.ProgressReadinessFailure:
		if resourcesCleanedUp(*resources) {
			return ReadinessFailedClean, resources, nil
		}
		return ReadinessFailedDirty, resources, nil

	case api.ProgressReady:
		// Deleted resources
		if !resourcesExist(*resources) {
			return SessionResourcesFailure, resources, nil
		}
		// Failed resources
		if resourcesFailed(*resources) {
			return SessionResourcesFailure, resources, nil
		}
		if resourcesReady(*resources) {
			return SessionRunning, resources, nil
		}
		// NOTE: this state should not be possible
		return None, nil, fmt.Errorf("Invalid state. Resources should be failed or ready in ready state")
	case api.ProgressSessionFailure:
		if resourcesCleanedUp(*resources) {
			return SessionFailedClean, resources, nil
		}
		return SessionFailedDirty, resources, nil
	}
	return None, nil, fmt.Errorf("Invalid state. Unknown state progress: %s", dmSession.Status.Progress)
}

type resources struct {
	pod          *corev1.Pod
	podReadiness *readiness
	service      *corev1.Service
	needService  bool
}

func resourcesEmpty(resources resources) bool {
	return resources.pod == nil && resources.service == nil
}

func resourcesCleanedUp(resources resources) bool {
	if resourcesEmpty(resources) {
		return true
	}
	if resources.service != nil {
		return false
	}
	if resources.pod != nil {
		return !podAlive(*resources.pod)
	}
	return true
}

func resourcesExist(resources resources) bool {
	serviceOk := !resources.needService || resources.service != nil
	return resources.pod != nil && serviceOk
}

func resourcesReady(resources resources) bool {
	return resourcesExist(resources) && resources.podReadiness != nil && resources.podReadiness.ready
}

func resourcesFailed(resources resources) bool {
	// TODO: do we care if service is ready or not?
	// TODO: are there any other conditions we can consider as "failure"?
	return resources.pod != nil && podFailed(*resources.pod)
	// NOTE: alternative approach: checking if at least one of the containers failed
	// return anyContainerFailed(pod)

	// NOTE: alternative approach: checking that the main container failed
	// return mainContainerFailed(pod)
}

func podFailed(pod corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodFailed
}

func podAlive(pod corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodPending
}

// func anyContainerFailed(pod corev1.Pod) bool {
// 	for _, contStatus := range pod.Status.ContainerStatuses {
// 		if contStatus.State.Terminated != nil {
// 			return contStatus.State.Terminated.ExitCode != 0
// 		}
// 	}
// 	return false
// }

// func mainContainerFailed(pod corev1.Pod) bool {
// 	for _, contStatus := range pod.Status.ContainerStatuses {
// 		if contStatus.Name == defaultContainerName && contStatus.State.Terminated != nil {
// 			return contStatus.State.Terminated.ExitCode != 0
// 		}
// 	}
// 	return false
// }

func (r *DatamoverSessionReconciler) getResources(ctx context.Context, dmSession *api.DatamoverSession) (*resources, error) {
	pod, err := r.getPod(ctx, dmSession)
	if err != nil {
		return nil, err
	}
	var podReadiness *readiness
	if pod != nil {
		podReadiness, err = r.getReadiness(ctx, *pod)
		if err != nil {
			return nil, err
		}
	}
	// TODO: service readiness
	needService := len(dmSession.Spec.LifecycleConfig.ServicePorts) > 0
	var service *corev1.Service
	if needService {
		service, err = r.getService(ctx, dmSession)
		if err != nil {
			return nil, err
		}
	}

	return &resources{
		pod:          pod,
		podReadiness: podReadiness,
		service:      service,
		needService:  needService,
	}, nil
}

func (r *DatamoverSessionReconciler) getPod(ctx context.Context, dmSession *api.DatamoverSession) (*corev1.Pod, error) {

	namespace := dmSession.Namespace

	log.Log.Info("Check if the pod resource exists.")

	podList := &corev1.PodList{}
	opts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{datamoverSessionLabel: dmSession.Name},
	}
	err := r.List(ctx, podList, opts...)
	// NOTE: we assume that empty list is returned if there are no error if there are no matching pods
	if err != nil {
		return nil, err
	}

	matchingPods := []corev1.Pod{}
	for _, pod := range podList.Items {
		if isOwnedBy(&pod, *dmSession) {
			matchingPods = append(matchingPods, pod)
		} else {
			// FIXME: emit event
			log.Log.Info("Found pod not matching owner reference of the session", "podName", pod.Name, "namespace", pod.Namespace)
		}
	}

	switch len(matchingPods) {
	case 0:
		return nil, nil
	case 1:
		return &matchingPods[0], nil
	default:
		return nil, fmt.Errorf("found multiple pods for %s", dmSession.Name)
	}
}

func (r *DatamoverSessionReconciler) getService(ctx context.Context, dmSession *api.DatamoverSession) (*corev1.Service, error) {
	namespace := dmSession.Namespace
	serviceName := GetServiceName(*dmSession)
	svc := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: namespace}, svc)
	if err == nil {
		log.Log.Info("Service resource exists.")
		if isOwnedBy(svc, *dmSession) {
			return svc, nil
		} else {
			return nil, fmt.Errorf("Found service not matching owner reference of the session. Service %s in namespace %s, session %s", serviceName, namespace, dmSession.Name)
		}
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}

	log.Log.Info("Service resource not found.")
	return nil, nil
}

type readiness struct {
	ready bool
	data  string
}

func (r *DatamoverSessionReconciler) getReadiness(ctx context.Context, pod corev1.Pod) (*readiness, error) {
	// NOTE: this behaviour assumes that we must have the data (at least empty data)
	// in order to consider pod to be ready
	if isPodReady(pod) {
		data, err := r.fetchSessionData(ctx, pod)
		if err != nil {
			return nil, err
		}
		if data != nil {
			return &readiness{
				ready: true,
				data:  *data,
			}, nil
		}
	}
	return &readiness{
		ready: false,
	}, nil
}

func isOwnedBy(controlled metav1.Object, owner api.DatamoverSession) bool {
	controller := metav1.GetControllerOf(controlled)
	log.Log.Info("Resource controller", "controller", controller)
	if controller != nil {
		controllerGV, err := schema.ParseGroupVersion(controller.APIVersion)
		if err != nil {
			return false
		}

		return controllerGV.Group == api.GroupVersion.Group &&
			controllerGV.Version == api.GroupVersion.Version &&
			controller.Kind == api.DatamoverSessionKind &&
			controller.Name == owner.Name &&
			controller.UID == owner.UID
	}
	return false
}
