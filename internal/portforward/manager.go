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
	Namespace       string
	Service         string
	RemotePort      int
	LocalPort       int
	StopChan        chan struct{}
	ReadyChan       chan struct{}
	StartedAt       time.Time
	ForwardingState k8s.ForwardingState
	FailureReason   string
	FailureTime     time.Time
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
	key := fmt.Sprintf("%s/%s:%d", namespace, serviceName, remotePort)
	
	// Check if already exists with minimal lock time
	m.mu.Lock()
	if _, exists := m.forwards[key]; exists {
		m.mu.Unlock()
		return 0, fmt.Errorf("port forwarding already active for %s", key)
	}
	m.mu.Unlock()

	// Create a basic ForwardInfo entry to track any failures that occur early
	fw := &ForwardInfo{
		Namespace:       namespace,
		Service:         serviceName,
		RemotePort:      remotePort,
		LocalPort:       0, // Will be set if we get a port
		StartedAt:       time.Now(),
		ForwardingState: k8s.ForwardingStatePending,
	}

	// Do the Kubernetes API calls without holding the mutex
	pods, err := m.client.GetClientset().CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", serviceName),
	})
	if err != nil {
		fw.ForwardingState = k8s.ForwardingStateFailed
		fw.FailureReason = fmt.Sprintf("Failed to list pods: %v", err)
		fw.FailureTime = time.Now()
		
		// Store the failed forward info with lock
		m.mu.Lock()
		m.forwards[key] = fw
		m.mu.Unlock()
		return 0, fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		endpoints, err := m.client.GetClientset().CoreV1().Endpoints(namespace).Get(ctx, serviceName, metav1.GetOptions{})
		if err != nil {
			fw.ForwardingState = k8s.ForwardingStateFailed
			fw.FailureReason = fmt.Sprintf("Failed to get endpoints: %v", err)
			fw.FailureTime = time.Now()
			
			m.mu.Lock()
			m.forwards[key] = fw
			m.mu.Unlock()
			return 0, fmt.Errorf("failed to get endpoints: %w", err)
		}

		if len(endpoints.Subsets) == 0 || len(endpoints.Subsets[0].Addresses) == 0 {
			fw.ForwardingState = k8s.ForwardingStateFailed
			fw.FailureReason = fmt.Sprintf("No pods found for service %s", serviceName)
			fw.FailureTime = time.Now()
			
			m.mu.Lock()
			m.forwards[key] = fw
			m.mu.Unlock()
			return 0, fmt.Errorf("no pods found for service %s", serviceName)
		}

		address := endpoints.Subsets[0].Addresses[0]
		if address.TargetRef == nil {
			fw.ForwardingState = k8s.ForwardingStateFailed
			fw.FailureReason = fmt.Sprintf("No target reference found for service %s", serviceName)
			fw.FailureTime = time.Now()
			
			m.mu.Lock()
			m.forwards[key] = fw
			m.mu.Unlock()
			return 0, fmt.Errorf("no target reference found for service %s", serviceName)
		}
		
		podName := address.TargetRef.Name
		if podName == "" {
			fw.ForwardingState = k8s.ForwardingStateFailed
			fw.FailureReason = fmt.Sprintf("Empty pod name for service %s", serviceName)
			fw.FailureTime = time.Now()
			
			m.mu.Lock()
			m.forwards[key] = fw
			m.mu.Unlock()
			return 0, fmt.Errorf("empty pod name for service %s", serviceName)
		}
		
		pod, err := m.client.GetClientset().CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			fw.ForwardingState = k8s.ForwardingStateFailed
			fw.FailureReason = fmt.Sprintf("Failed to get pod: %v", err)
			fw.FailureTime = time.Now()
			
			m.mu.Lock()
			m.forwards[key] = fw
			m.mu.Unlock()
			return 0, fmt.Errorf("failed to get pod: %w", err)
		}
		pods.Items = []corev1.Pod{*pod}
	}

	if len(pods.Items) == 0 {
		fw.ForwardingState = k8s.ForwardingStateFailed
		fw.FailureReason = fmt.Sprintf("No pods available for service %s", serviceName)
		fw.FailureTime = time.Now()
		
		m.mu.Lock()
		m.forwards[key] = fw
		m.mu.Unlock()
		return 0, fmt.Errorf("no pods available for service %s", serviceName)
	}
	
	podName := pods.Items[0].Name
	if podName == "" {
		fw.ForwardingState = k8s.ForwardingStateFailed
		fw.FailureReason = fmt.Sprintf("Pod name is empty for service %s", serviceName)
		fw.FailureTime = time.Now()
		
		m.mu.Lock()
		m.forwards[key] = fw
		m.mu.Unlock()
		return 0, fmt.Errorf("pod name is empty for service %s", serviceName)
	}

	var localPort int
	if preferredLocalPort > 0 {
		// Check if the preferred port is available
		if isPortAvailable(preferredLocalPort) {
			localPort = preferredLocalPort
		} else {
			fw.ForwardingState = k8s.ForwardingStateFailed
			fw.FailureReason = fmt.Sprintf("Port %d is already in use", preferredLocalPort)
			fw.FailureTime = time.Now()
			
			m.mu.Lock()
			m.forwards[key] = fw
			m.mu.Unlock()
			return 0, fmt.Errorf("port %d is already in use", preferredLocalPort)
		}
	} else {
		// Get any available port
		localPort, err = getFreePort()
		if err != nil {
			fw.ForwardingState = k8s.ForwardingStateFailed
			fw.FailureReason = fmt.Sprintf("Failed to get free port: %v", err)
			fw.FailureTime = time.Now()
			
			m.mu.Lock()
			m.forwards[key] = fw
			m.mu.Unlock()
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

	// Update the existing fw instance
	fw.LocalPort = localPort
	fw.StopChan = stopChan
	fw.ReadyChan = readyChan

	ports := []string{fmt.Sprintf("%d:%d", localPort, remotePort)}

	go func() {
		defer func() {
			// Ensure we clean up if something goes wrong
			if r := recover(); r != nil {
				// Panic occurred, ensure channels are closed
				select {
				case <-readyChan:
					// Already closed
				default:
					close(readyChan)
				}
			}
		}()
		
		// Use a buffer to capture any critical errors while discarding normal output
		out, errOut := io.Discard, io.Discard
		pf, err := portforward.NewOnAddresses(dialer, []string{"localhost"}, ports, stopChan, readyChan, out, errOut)
		if err != nil {
			// Critical error - mark as failed and close readyChan to signal failure
			m.mu.Lock()
			fw.ForwardingState = k8s.ForwardingStateFailed
			fw.FailureReason = fmt.Sprintf("Failed to create port forwarder: %v", err)
			fw.FailureTime = time.Now()
			m.mu.Unlock()
			
			select {
			case <-readyChan:
				// Already closed
			default:
				close(readyChan)
			}
			return
		}
		
		// Run port forwarding in a separate goroutine with monitoring
		errChan := make(chan error, 1)
		go func() {
			errChan <- pf.ForwardPorts()
		}()
		
		// Monitor for errors or stop signal
		select {
		case err := <-errChan:
			if err != nil {
				// Port forwarding failed
				m.mu.Lock()
				if fw, exists := m.forwards[key]; exists {
					fw.ForwardingState = k8s.ForwardingStateFailed
					fw.FailureReason = fmt.Sprintf("Port forwarding failed: %v", err)
					fw.FailureTime = time.Now()
				}
				m.mu.Unlock()
			}
		case <-stopChan:
			// Normal stop requested
			return
		}
	}()

	// Store forward info immediately as pending with lock
	m.mu.Lock()
	m.forwards[key] = fw
	m.mu.Unlock()
	
	select {
	case <-readyChan:
		// Mark as ready
		m.mu.Lock()
		fw.ForwardingState = k8s.ForwardingStateActive
		m.mu.Unlock()
		return localPort, nil
	case <-ctx.Done():
		m.mu.Lock()
		fw.ForwardingState = k8s.ForwardingStateFailed
		fw.FailureReason = "Context cancelled"
		fw.FailureTime = time.Now()
		m.mu.Unlock()
		close(stopChan)
		return 0, fmt.Errorf("context cancelled")
	case <-time.After(10 * time.Second):
		m.mu.Lock()
		fw.ForwardingState = k8s.ForwardingStateFailed
		fw.FailureReason = "Timeout waiting for port forward to be ready"
		fw.FailureTime = time.Now()
		m.mu.Unlock()
		close(stopChan)
		return 0, fmt.Errorf("timeout waiting for port forward to be ready")
	}
}

func (m *Manager) StopForward(namespace, serviceName string, remotePort int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s/%s:%d", namespace, serviceName, remotePort)
	if fw, exists := m.forwards[key]; exists {
		// Safely close the channel if it exists and is not already closed
		if fw.StopChan != nil {
			select {
			case <-fw.StopChan:
				// Already closed
			default:
				close(fw.StopChan)
			}
		}
		delete(m.forwards, key)
	}
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, fw := range m.forwards {
		// Safely close the channel if it exists and is not already closed
		if fw.StopChan != nil {
			select {
			case <-fw.StopChan:
				// Already closed
			default:
				close(fw.StopChan)
			}
		}
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

// GetForwardInfo returns the forward info for a specific port
func (m *Manager) GetForwardInfo(namespace, serviceName string, remotePort int) *ForwardInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := fmt.Sprintf("%s/%s:%d", namespace, serviceName, remotePort)
	if fw, exists := m.forwards[key]; exists {
		// Return a copy to avoid race conditions
		copy := *fw
		return &copy
	}
	return nil
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
