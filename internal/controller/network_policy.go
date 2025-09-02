package controller

import (
	"context"

	api "github.com/kanisterio/datamover/api/v1alpha1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *DatamoverSessionReconciler) CreateNetworkPolicy(ctx context.Context, dmSession api.DatamoverSession) error {
	if len(dmSession.Spec.LifecycleConfig.ServicePorts) < 1 {
		return errors.New("ServicePorts should be set to create a network policy")
	}
	if !dmSession.Spec.LifecycleConfig.NetworkPolicy.Enabled {
		return errors.New("NetworkPolicy is disabled")
	}

	np := makeNetworkPolicySpec(dmSession)

	if err := controllerutil.SetControllerReference(&dmSession, &np, r.Scheme); err != nil {
		return err
	}

	err := r.Create(ctx, &np)
	log.Log.Info("Created network policy.")
	if err != nil {
		return errors.Wrap(err, "Failed to create network policy")
	}
	// TODO: Wait for policy to be created???
	return nil
}

func (r *DatamoverSessionReconciler) DeleteNetworkPolicy(ctx context.Context, service *corev1.Service) error {
	return r.Delete(ctx, service)
}

func makeNetworkPolicySpec(dmSession api.DatamoverSession) networkingv1.NetworkPolicy {
	return networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dmSession.Name,
			Namespace: dmSession.Namespace,
			Labels: map[string]string{
				"name":                            dmSession.Name,
				api.DatamoverSessionLabel:         dmSession.Name,
				api.DatamoverSessionSelectorLabel: GetServiceName(dmSession),
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					api.DatamoverSessionSelectorLabel: dmSession.Name,
					api.DatamoverSessionLabel:         dmSession.Name,
				},
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From:  dmSession.Spec.LifecycleConfig.NetworkPolicy.From,
					Ports: networkPolicyPorts(dmSession),
				},
			},
		},
	}
}

func networkPolicyPorts(dmSession api.DatamoverSession) []networkingv1.NetworkPolicyPort {
	ports := dmSession.Spec.LifecycleConfig.ServicePorts
	policyPorts := []networkingv1.NetworkPolicyPort{}
	for _, port := range ports {
		servicePort := intstr.FromInt32(port.Port)
		policyPorts = append(policyPorts, networkingv1.NetworkPolicyPort{
			Protocol: &port.Protocol,
			Port:     &servicePort,
		})
	}
	return policyPorts
}
