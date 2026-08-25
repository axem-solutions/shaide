package kube

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type ForwardTarget struct {
	PodName    string
	TargetPort int
}

type ServiceRef struct {
	ctx         context.Context
	k8sclient   kubernetes.Interface
	Namespace   string
	ServiceName string
}

func NewServiceRef(ctx context.Context, k8sclient kubernetes.Interface, namespace, serviceName string) *ServiceRef {
	return &ServiceRef{
		ctx:         ctx,
		k8sclient:   k8sclient,
		Namespace:   namespace,
		ServiceName: serviceName,
	}
}

func (s *ServiceRef) ForwardTarget() (*ForwardTarget, error) {
	target := &ForwardTarget{}

	if err := target.checkServiceExists(s); err != nil {
		return nil, err
	}

	if err := target.checkServiceEndpointSlices(s); err != nil {
		return nil, err
	}
	return target, nil
}

func (t *ForwardTarget) checkServiceExists(s *ServiceRef) error {
	svc, err := s.k8sclient.CoreV1().Services(s.Namespace).Get(s.ctx, s.ServiceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}
	return t.validateService(svc)
}

func (t *ForwardTarget) validateService(svc *v1.Service) error {
	if len(svc.Spec.Selector) == 0 {
		return fmt.Errorf("service %q has no selector — cannot port-forward", svc.Name)
	}
	if len(svc.Spec.Ports) == 0 {
		return fmt.Errorf("service %q has no ports defined", svc.Name)
	}

	t.TargetPort = svc.Spec.Ports[0].TargetPort.IntValue()
	if t.TargetPort == 0 {
		t.TargetPort = int(svc.Spec.Ports[0].Port)
	}

	return nil
}

func (t *ForwardTarget) checkServiceEndpointSlices(s *ServiceRef) error {
	slices, err := s.k8sclient.DiscoveryV1().EndpointSlices(s.Namespace).List(
		s.ctx,
		metav1.ListOptions{
			LabelSelector: fmt.Sprintf("kubernetes.io/service-name=%s", s.ServiceName),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to get endpoint slices for service %s: %w", s.ServiceName, err)
	}

	if err := t.validateServiceEndpointSlice(slices); err != nil {
		return fmt.Errorf("failed to validate service EndpointSlices %s: %w", s.ServiceName, err)
	}

	return nil
}

func (t *ForwardTarget) validateServiceEndpointSlice(slices *discoveryv1.EndpointSliceList) error {
	if len(slices.Items) == 0 {
		return fmt.Errorf("no endpoint slices found")
	}

	for _, slice := range slices.Items {
		for _, endpoint := range slice.Endpoints {
			if endpoint.Conditions.Ready == nil || !*endpoint.Conditions.Ready {
				continue
			}
			if endpoint.TargetRef == nil {
				continue
			}
			t.PodName = endpoint.TargetRef.Name
			return nil
		}
	}
	return fmt.Errorf("no ready endpoints")
}
