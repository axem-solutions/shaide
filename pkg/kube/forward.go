package kube

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

const (
	// A port-forward is a SPDY stream held open through the API server, and on
	// managed clusters the tunnel in front of it drops long-lived transfers.
	// Multi-GB blob uploads run far longer than such a stream reliably
	// survives, so the forward reconnects instead of leaving the caller with a
	// closed listener and a refused connection.
	defaultReconnectAttempts = 10
	defaultReconnectDelay    = 2 * time.Second
	maxReconnectDelay        = 30 * time.Second
	reconnectTimeout         = 30 * time.Second
)

func StartPortForward(
	ctx context.Context,
	restCfg *rest.Config,
	client kubernetes.Interface,
	target ForwardRequest,
) (*Forward, error) {
	session, err := startForwardSession(ctx, restCfg, client, target)
	if err != nil {
		return nil, err
	}

	forward := &Forward{
		localPort: session.localPort,
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
		logf:      target.Logf,
	}

	// Later sessions must land on the same local port: the callers that hold
	// this address have already handed it to an HTTP client, and a reconnect
	// that moved the port would silently stop serving them.
	target.LocalPort = int(session.localPort)

	// The caller's context carries the dial deadline for the first connect and
	// is cancelled as soon as it returns. Reconnects outlive that by design, so
	// the supervisor keeps the values and drops the cancellation, and bounds
	// each attempt with a deadline of its own.
	go forward.supervise(context.WithoutCancel(ctx), restCfg, client, target, session)

	return forward, nil
}

type forwardSession struct {
	localPort uint16
	stopCh    chan struct{}
	doneCh    chan struct{}
	errCh     chan error
}

func (s *forwardSession) close() {
	close(s.stopCh)
	<-s.doneCh
}

func startForwardSession(
	ctx context.Context,
	restCfg *rest.Config,
	client kubernetes.Interface,
	target ForwardRequest,
) (*forwardSession, error) {
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

	fail := func(err error) (*forwardSession, error) {
		close(stopCh)
		<-doneCh
		return nil, err
	}

	select {
	case <-readyCh:
	case err := <-errCh:
		return fail(fmt.Errorf("start port-forward: %w", err))
	case <-ctx.Done():
		return fail(fmt.Errorf("wait for port forward: %w", ctx.Err()))
	}

	ports, err := forwarder.GetPorts()
	if err != nil {
		return fail(fmt.Errorf("read forwarded ports: %w", err))
	}
	if len(ports) != 1 {
		return fail(fmt.Errorf("expected 1 forwarded port, got %d", len(ports)))
	}

	return &forwardSession{
		localPort: ports[0].Local,
		stopCh:    stopCh,
		doneCh:    doneCh,
		errCh:     errCh,
	}, nil
}

type Forward struct {
	localPort uint16
	stopCh    chan struct{}
	doneCh    chan struct{}
	closeOnce sync.Once

	logf func(format string, args ...any)
}

type ForwardRequest struct {
	Namespace  string
	Service    string
	PodName    string
	LocalPort  int
	RemotePort int

	// Logf receives reconnect messages. Optional.
	Logf func(format string, args ...any)
}

// supervise replaces the session whenever it ends on its own. ForwardPorts
// returning takes the local listener with it, so without this the first
// symptom of a dropped tunnel is a refused connection on the next request,
// with no way back short of restarting the step.
func (f *Forward) supervise(
	ctx context.Context,
	restCfg *rest.Config,
	client kubernetes.Interface,
	target ForwardRequest,
	session *forwardSession,
) {
	defer close(f.doneCh)

	for {
		select {
		case <-f.stopCh:
			session.close()
			return

		case <-ctx.Done():
			session.close()
			return

		case <-session.doneCh:
			// The forwarder exited. Closing was requested concurrently in the
			// race where both fire, and then there is nothing to reconnect.
			select {
			case <-f.stopCh:
				return
			default:
			}

			f.log("port-forward %s:%d dropped: %v", target.PodName, target.LocalPort, sessionError(session))

			next, err := f.reconnect(ctx, restCfg, client, target)
			if err != nil {
				f.log("port-forward %s:%d could not be re-established: %v", target.PodName, target.LocalPort, err)
				return
			}

			f.log("port-forward %s:%d re-established", target.PodName, target.LocalPort)
			session = next
		}
	}
}

func (f *Forward) reconnect(
	ctx context.Context,
	restCfg *rest.Config,
	client kubernetes.Interface,
	target ForwardRequest,
) (*forwardSession, error) {
	delay := defaultReconnectDelay

	var lastErr error
	for attempt := 1; attempt <= defaultReconnectAttempts; attempt++ {
		if err := f.wait(ctx, delay); err != nil {
			return nil, err
		}

		session, err := f.connect(ctx, restCfg, client, target)
		if err == nil {
			return session, nil
		}

		lastErr = err
		f.log("port-forward reconnect attempt %d/%d failed: %v", attempt, defaultReconnectAttempts, err)

		if delay < maxReconnectDelay {
			delay *= 2
		}
	}

	return nil, fmt.Errorf("after %d attempts: %w", defaultReconnectAttempts, lastErr)
}

func (f *Forward) connect(
	ctx context.Context,
	restCfg *rest.Config,
	client kubernetes.Interface,
	target ForwardRequest,
) (*forwardSession, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, reconnectTimeout)
	defer cancel()

	return startForwardSession(attemptCtx, restCfg, client, target)
}

func (f *Forward) wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-f.stopCh:
		return fmt.Errorf("port-forward closed")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *Forward) log(format string, args ...any) {
	if f.logf == nil {
		return
	}

	f.logf(format, args...)
}

func sessionError(session *forwardSession) error {
	select {
	case err := <-session.errCh:
		return err
	default:
		return fmt.Errorf("connection closed")
	}
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
