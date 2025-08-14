package portforward

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/grumpylabs/kpf/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

type ForwardInfo struct {
	Namespace  string
	Service    string
	RemotePort int
	LocalPort  int
	StopChan   chan struct{}
	ReadyChan  chan struct{}
}

type Manager struct {
	client   *k8s.Client
	forwards map[string]*ForwardInfo
	mu       sync.RWMutex
}

func NewManager(client *k8s.Client) *Manager {
	return &Manager{
		client:   client,
		forwards: make(map[string]*ForwardInfo),
	}
}

func (m *Manager) StartForward(ctx context.Context, namespace, serviceName string, remotePort int) (int, error) {
	return m.StartForwardWithLocalPort(ctx, namespace, serviceName, remotePort, 0)
}

func (m *Manager) StartForwardWithLocalPort(ctx context.Context, namespace, serviceName string, remotePort, preferredLocalPort int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s/%s:%d", namespace, serviceName, remotePort)
	if _, exists := m.forwards[key]; exists {
		return 0, fmt.Errorf("port forwarding already active for %s", key)
	}

	pods, err := m.client.GetClientset().CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", serviceName),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		endpoints, err := m.client.GetClientset().CoreV1().Endpoints(namespace).Get(ctx, serviceName, metav1.GetOptions{})
		if err != nil {
			return 0, fmt.Errorf("failed to get endpoints: %w", err)
		}

		if len(endpoints.Subsets) == 0 || len(endpoints.Subsets[0].Addresses) == 0 {
			return 0, fmt.Errorf("no pods found for service %s", serviceName)
		}

		podName := endpoints.Subsets[0].Addresses[0].TargetRef.Name
		pod, err := m.client.GetClientset().CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return 0, fmt.Errorf("failed to get pod: %w", err)
		}
		pods.Items = []corev1.Pod{*pod}
	}

	podName := pods.Items[0].Name

	var localPort int
	if preferredLocalPort > 0 {
		// Check if the preferred port is available
		if isPortAvailable(preferredLocalPort) {
			localPort = preferredLocalPort
		} else {
			return 0, fmt.Errorf("port %d is already in use", preferredLocalPort)
		}
	} else {
		// Get any available port
		localPort, err = getFreePort()
		if err != nil {
			return 0, fmt.Errorf("failed to get free port: %w", err)
		}
	}

	req := m.client.GetClientset().CoreV1().RESTClient().
		Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("portforward")

	transport, upgrader, err := spdy.RoundTripperFor(m.client.GetConfig())
	if err != nil {
		return 0, fmt.Errorf("failed to create round tripper: %w", err)
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", req.URL())

	stopChan := make(chan struct{}, 1)
	readyChan := make(chan struct{})

	fw := &ForwardInfo{
		Namespace:  namespace,
		Service:    serviceName,
		RemotePort: remotePort,
		LocalPort:  localPort,
		StopChan:   stopChan,
		ReadyChan:  readyChan,
	}

	ports := []string{fmt.Sprintf("%d:%d", localPort, remotePort)}

	go func() {
		// Use a buffer to capture any critical errors while discarding normal output
		out, errOut := io.Discard, io.Discard
		pf, err := portforward.NewOnAddresses(dialer, []string{"localhost"}, ports, stopChan, readyChan, out, errOut)
		if err != nil {
			// Critical error - close readyChan to signal failure
			close(readyChan)
			return
		}
		if err := pf.ForwardPorts(); err != nil {
			// Port forwarding failed - this is normal when stopping
			return
		}
	}()

	select {
	case <-readyChan:
		m.forwards[key] = fw
		return localPort, nil
	case <-ctx.Done():
		close(stopChan)
		return 0, fmt.Errorf("context cancelled")
	case <-time.After(10 * time.Second):
		close(stopChan)
		return 0, fmt.Errorf("timeout waiting for port forward to be ready")
	}
}

func (m *Manager) StopForward(namespace, serviceName string, remotePort int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s/%s:%d", namespace, serviceName, remotePort)
	if fw, exists := m.forwards[key]; exists {
		close(fw.StopChan)
		delete(m.forwards, key)
	}
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, fw := range m.forwards {
		close(fw.StopChan)
		delete(m.forwards, key)
	}
}

func (m *Manager) IsForwarding(namespace, serviceName string, remotePort int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := fmt.Sprintf("%s/%s:%d", namespace, serviceName, remotePort)
	_, exists := m.forwards[key]
	return exists
}

func (m *Manager) GetLocalPort(namespace, serviceName string, remotePort int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := fmt.Sprintf("%s/%s:%d", namespace, serviceName, remotePort)
	if fw, exists := m.forwards[key]; exists {
		return fw.LocalPort
	}
	return 0
}

// IsServiceForwarding checks if any port of a service is being forwarded
func (m *Manager) IsServiceForwarding(namespace, serviceName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	prefix := fmt.Sprintf("%s/%s:", namespace, serviceName)
	for key := range m.forwards {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// GetActiveForwards returns all active port forwards for a service
func (m *Manager) GetActiveForwards(namespace, serviceName string) []ForwardInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var forwards []ForwardInfo
	prefix := fmt.Sprintf("%s/%s:", namespace, serviceName)
	for key, fw := range m.forwards {
		if strings.HasPrefix(key, prefix) {
			forwards = append(forwards, *fw)
		}
	}
	return forwards
}

// Debug method to see all forwards
func (m *Manager) GetAllForwards() map[string]*ForwardInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make(map[string]*ForwardInfo)
	for k, v := range m.forwards {
		result[k] = v
	}
	return result
}

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func isPortAvailable(port int) bool {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return false
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return false
	}
	defer l.Close()
	return true
}
