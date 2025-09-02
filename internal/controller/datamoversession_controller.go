/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/kanisterio/datamover/api/v1alpha1"
)

// FIXME: domain for labels
const (
	datamoverSessionSelectorLabel = "datamover/service_label"
	datamoverSessionLabel         = "datamover/session"
)

// DatamoverSessionReconciler reconciles a DatamoverSession object
type DatamoverSessionReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	RestConfig rest.Config
	mgr        ctrl.Manager
}

// +kubebuilder:rbac:groups=dm.cr.kanister.io,resources=datamoversessions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dm.cr.kanister.io,resources=datamoversessions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dm.cr.kanister.io,resources=datamoversessions/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=*
// +kubebuilder:rbac:groups="",resources=pods/ephemeralcontainers,verbs=*
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=*
// +kubebuilder:rbac:groups="",resources=services,verbs=*
// +kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=*

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DatamoverSession object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *DatamoverSessionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	session := &api.DatamoverSession{}
	if err := r.Get(ctx, req.NamespacedName, session); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.Log.Info("Read session resource", "status", session.Status)
	// Handle only lifecycle sessions
	if session.Spec.LifecycleConfig != nil {
		return r.Run(ctx, session)
	} else {
		// Skip non-lifecycle sessions
		return ctrl.Result{}, nil
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatamoverSessionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.mgr = mgr
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.DatamoverSession{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}
