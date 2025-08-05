package controller

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	api "github.com/kanisterio/datamover/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"

	reconciler "github.com/kanisterio/datamover/internal/controller"
)

func MakeControllerManager(restConfig *rest.Config, options ctrl.Options) (manager.Manager, error) {
	if options.Scheme == nil {
		scheme, err := makeScheme()
		if err != nil {
			return nil, err
		}
		options.Scheme = scheme
	}
	mgr, err := ctrl.NewManager(restConfig, options)
	if err != nil {
		log.Log.Error(err, "unable to start manager")
		return nil, err
	}

	if err = (&reconciler.DatamoverSessionReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		RestConfig: *restConfig,
	}).SetupWithManager(mgr); err != nil {
		log.Log.Error(err, "unable to create controller", "controller", "DatamoverSession")
		return nil, err
	}
	// +kubebuilder:scaffold:builder

	return mgr, nil
}

func makeScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := api.AddToScheme(scheme); err != nil {
		return nil, err
	}
	// +kubebuilder:scaffold:scheme
	return scheme, nil
}
