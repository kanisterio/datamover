package controller

import (
	"context"
	api "github.com/kanisterio/datamover/api/v1alpha1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *DatamoverSessionReconciler) CreateService(ctx context.Context, dmSession api.DatamoverSession) error {
	if len(dmSession.Spec.LifecycleConfig.ServicePorts) < 1 {
		return errors.New("ServicePorts shoudl be set to create service")
	}
	serviceName := GetServiceName(dmSession)
	svc := makeServiceSpec(dmSession, serviceName)

	if err := controllerutil.SetControllerReference(&dmSession, &svc, r.Scheme); err != nil {
		return err
	}

	err := r.Create(ctx, &svc)
	log.Log.Info("Created service.")
	if err != nil {
		// TODO: wrap error
		return errors.Wrap(err, "Failed to create service")
	}
	// TODO: Wait for service to be created???
	return nil
}

func (r *DatamoverSessionReconciler) DeleteService(ctx context.Context, service *corev1.Service) error {
	return r.Delete(ctx, service)
}

func makeServiceSpec(dmSession api.DatamoverSession, serviceName string) corev1.Service {
	ports := dmSession.Spec.LifecycleConfig.ServicePorts
	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: dmSession.Namespace,
			// TODO: use const here??
			// TODO: do we want to make labels configurable
			Labels: map[string]string{
				"name":                serviceName,
				datamoverSessionLabel: dmSession.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports:    ports,
			Selector: map[string]string{datamoverSessionSelectorLabel: dmSession.Name},
		},
	}
}

func GetServiceName(dmSession api.DatamoverSession) string {
	// Deterministic get. Currently based on session name.
	// TODO: if we need to generate service name,
	// this function should NOT be used to return prefix,
	// but the generated name instead.
	return dmSession.Name + "-service"
}
