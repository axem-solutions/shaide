package kube

import (
	"context"
	"fmt"
	"hash/fnv"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/axem-solutions/ai_platform/pkg/kube/connection"
)

// Environment variables that trigger and configure the detached port-forward
// daemon (see EnsureDurableForward). Deliberately namespaced and never meant
// to be set by hand — only spawnDaemon below ever sets them.
const (
	daemonTriggerEnv    = "__AIPLATFORM_KUBE_PF_DAEMON"
	daemonKubeconfigEnv = "__AIPLATFORM_KUBE_PF_KUBECONFIG"
	daemonContextEnv    = "__AIPLATFORM_KUBE_PF_CONTEXT"
	daemonNamespaceEnv  = "__AIPLATFORM_KUBE_PF_NAMESPACE"
	daemonServiceEnv    = "__AIPLATFORM_KUBE_PF_SERVICE"
	daemonLocalPortEnv  = "__AIPLATFORM_KUBE_PF_LOCAL_PORT"

	// daemonLifetime bounds how long a spawned daemon keeps its port-forward
	// open before exiting on its own — bounded rather than tracked by
	// activity, so an abandoned daemon can never accumulate forever. A later
	// operation within this window reuses it; after it expires, the next one
	// just pays a fresh ~1s startup cost.
	daemonLifetime = 30 * time.Minute

	// daemonReadyTimeout is how long EnsureDurableForward waits for a newly
	// spawned daemon to start actually responding before giving up.
	daemonReadyTimeout = 15 * time.Second
)

// init runs before main() in every binary that imports this package — this
// is what lets a daemon spawned via spawnDaemon's re-exec turn into the
// forwarding process itself, regardless of which binary originally spawned
// it (the standalone cloud-harbor program, or the installer running this
// inline via the Automation API). Same trampoline pattern as Docker's
// pkg/reexec. Only ever triggers if daemonTriggerEnv is set, which nothing
// but spawnDaemon ever sets.
func init() {
	if os.Getenv(daemonTriggerEnv) == "" {
		return
	}
	runDaemon()
	os.Exit(0)
}

// EnsureDurableForward returns a local address forwarding to namespace/service
// on the cluster reached via kubeconfigPath/contextName, backed by a
// port-forward that outlives the calling process.
//
// That durability is the whole point: Pulumi's engine can perform provider
// operations — including deletes, during `pulumi destroy` — well after the
// program that registered the resources has already exited (registering the
// resource graph and actually executing every operation against it are
// separate phases; the calling process doesn't stay alive through both). A
// port-forward that's just a goroutine inside that process would already be
// gone by delete time. This keeps the forward alive independently, in its
// own detached process, reused across invocations for up to daemonLifetime.
func EnsureDurableForward(kubeconfigPath, contextName, namespace, service string) (string, error) {
	localPort := derivePort(kubeconfigPath, contextName, namespace, service)
	addr := fmt.Sprintf("127.0.0.1:%d", localPort)

	if daemonRespondingAt(addr) {
		return "http://" + addr, nil
	}

	if err := spawnDaemon(kubeconfigPath, contextName, namespace, service, localPort); err != nil {
		return "", fmt.Errorf("spawn port-forward daemon: %w", err)
	}

	deadline := time.Now().Add(daemonReadyTimeout)
	for time.Now().Before(deadline) {
		if daemonRespondingAt(addr) {
			return "http://" + addr, nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	return "", fmt.Errorf("port-forward daemon on %s did not become ready within %s", addr, daemonReadyTimeout)
}

// derivePort picks a deterministic local port for this specific
// cluster/namespace/service combination, so a later invocation (pulumi
// destroy run minutes after pulumi up) can find and reuse the same daemon
// without needing any separate state file to look it up.
func derivePort(kubeconfigPath, contextName, namespace, service string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(kubeconfigPath + "|" + contextName + "|" + namespace + "|" + service))
	const base = 30000
	const span = 10000
	return base + int(h.Sum32()%span)
}

// daemonRespondingAt checks not just that something is listening on addr,
// but that it's genuinely proxying to the expected service — distinguishing
// a live daemon of ours from an unrelated process that happens to occupy the
// same deterministically-derived port. Assumes an HTTP service on the other
// end, true for every current caller (Harbor's API).
func daemonRespondingAt(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false
	}
	conn.Close()

	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + addr + "/")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return true
}

func spawnDaemon(kubeconfigPath, contextName, namespace, service string, localPort int) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve own executable path: %w", err)
	}

	cmd := exec.Command(exePath)
	cmd.Env = append(os.Environ(),
		daemonTriggerEnv+"=1",
		daemonKubeconfigEnv+"="+kubeconfigPath,
		daemonContextEnv+"="+contextName,
		daemonNamespaceEnv+"="+namespace,
		daemonServiceEnv+"="+service,
		daemonLocalPortEnv+"="+strconv.Itoa(localPort),
	)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return err
	}
	// Deliberately not cmd.Wait()'d — the whole point is that this process
	// keeps running after the caller (and its process tree) exits.
	return nil
}

// runDaemon is the daemon process's entire job: hold one port-forward open
// until daemonLifetime elapses, then exit (which tears the forward down
// along with it). Only ever reached via the init() trampoline above.
func runDaemon() {
	kubeconfigPath := os.Getenv(daemonKubeconfigEnv)
	contextName := os.Getenv(daemonContextEnv)

	namespace := os.Getenv(daemonNamespaceEnv)
	service := os.Getenv(daemonServiceEnv)
	localPort, err := strconv.Atoi(os.Getenv(daemonLocalPortEnv))
	if err != nil {
		return
	}

	clientset, restCfg, err := connection.NewK8sClient(connection.Connection{
		kubeconfigPath,
		contextName,
	})
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	target, err := NewServiceRef(ctx, clientset, namespace, service).ForwardTarget()
	if err != nil {
		return
	}

	forward, err := StartPortForward(ctx, restCfg, clientset, ForwardRequest{
		Namespace:  namespace,
		Service:    service,
		PodName:    target.PodName,
		LocalPort:  localPort,
		RemotePort: target.TargetPort,
	})
	if err != nil {
		return
	}
	defer forward.Close()

	time.Sleep(daemonLifetime)
}
