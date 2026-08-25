package kube

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

func StartPortForward(
	ctx context.Context,
	restCfg *rest.Config,
	client kubernetes.Interface,
	target ForwardRequest,
) (*Forward, error) {
	url := client.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Namespace(target.Namespace).
		Name(target.PodName).
		SubResource("portforward").
		URL()

	roundTripper, upgrader, err := spdy.RoundTripperFor(restCfg)
	if err != nil {
		return nil, fmt.Errorf("build spdy transport: %w", err)
	}

	dialer := spdy.NewDialer(
		upgrader,
		&http.Client{Transport: roundTripper},
		http.MethodPost,
		url,
	)

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	errCh := make(chan error, 1)
	doneCh := make(chan struct{})

	forwarder, err := portforward.NewOnAddresses(
		dialer,
		[]string{"127.0.0.1"},
		[]string{fmt.Sprintf("%d:%d", target.LocalPort, target.RemotePort)},
		stopCh,
		readyCh,
		io.Discard,
		io.Discard,
	)

	if err != nil {
		return nil, fmt.Errorf("create port forwarder: %w", err)
	}

	go func() {
		defer close(doneCh)
		if err := forwarder.ForwardPorts(); err != nil {
			select {
			case errCh <- err:
			default:
			}
		}
	}()
	select {
	case <-readyCh:
	case err := <-errCh:
		close(stopCh)
		<-doneCh
		return nil, fmt.Errorf("start port-forward: %w", err)
	case <-ctx.Done():
		close(stopCh)
		<-doneCh
		return nil, fmt.Errorf("wait for port forward: %w", ctx.Err())
	}

	ports, err := forwarder.GetPorts()
	if err != nil {
		close(stopCh)
		<-doneCh
		return nil, fmt.Errorf("read forwarded ports: %w", err)
	}
	if len(ports) != 1 {
		close(stopCh)
		<-doneCh
		return nil, fmt.Errorf("expected 1 forwarded port, got %d", len(ports))
	}

	return &Forward{
		localPort: ports[0].Local,
		stopCh:    stopCh,
		doneCh:    doneCh,
	}, nil
}

type Forward struct {
	localPort uint16
	stopCh    chan struct{}
	doneCh    chan struct{}
	closeOnce sync.Once
}

type ForwardRequest struct {
	Namespace  string
	Service    string
	PodName    string
	LocalPort  int
	RemotePort int
}

func (f *Forward) LocalPort() uint16 {
	return f.localPort
}

func (f *Forward) Address() string {
	return fmt.Sprintf("http://127.0.0.1:%d", f.localPort)
}

func (f *Forward) Close() {
	f.closeOnce.Do(func() {
		close(f.stopCh)
		<-f.doneCh
	})
}
