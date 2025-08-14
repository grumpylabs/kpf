package k8s

import (
	"context"
	"fmt"
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type Client struct {
	clientset *kubernetes.Clientset
	config    *rest.Config
	namespace string
}

func NewClient(kubeconfig, namespace string) (*Client, error) {
	var config *rest.Config
	var err error

	if kubeconfig == "" {
		kubeconfig = filepath.Join(homedir.HomeDir(), ".kube", "config")
	}

	config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create config: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return &Client{
		clientset: clientset,
		config:    config,
		namespace: namespace,
	}, nil
}

func (c *Client) GetServices(ctx context.Context) ([]ServiceInfo, error) {
	var services []ServiceInfo

	namespaces := []string{c.namespace}
	if c.namespace == "" {
		nsList, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to list namespaces: %w", err)
		}
		namespaces = make([]string, 0, len(nsList.Items))
		for _, ns := range nsList.Items {
			namespaces = append(namespaces, ns.Name)
		}
	}

	for _, ns := range namespaces {
		svcList, err := c.clientset.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to list services in namespace %s: %w", ns, err)
		}

		for _, svc := range svcList.Items {
			services = append(services, ServiceInfo{
				Name:      svc.Name,
				Namespace: svc.Namespace,
				Type:      string(svc.Spec.Type),
				Ports:     extractPorts(&svc),
				Service:   &svc,
			})
		}
	}

	return services, nil
}

func extractPorts(svc *corev1.Service) []PortInfo {
	var ports []PortInfo
	for _, port := range svc.Spec.Ports {
		ports = append(ports, PortInfo{
			Name:       port.Name,
			Port:       port.Port,
			TargetPort: port.TargetPort.IntVal,
			Protocol:   string(port.Protocol),
		})
	}
	return ports
}

func (c *Client) GetDeploymentForService(ctx context.Context, namespace, serviceName string) (*DeploymentInfo, error) {
	// First get the service to check its selector
	service, err := c.clientset.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// Get all deployments in the namespace
	deployments, err := c.clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	// Look for a deployment that matches the service
	for _, dep := range deployments.Items {
		// Check if deployment name matches service name
		if dep.Name == serviceName {
			var replicas int32 = 0
			if dep.Spec.Replicas != nil {
				replicas = *dep.Spec.Replicas
			}
			return &DeploymentInfo{
				Name:            dep.Name,
				Replicas:        replicas,
				ReadyReplicas:   dep.Status.ReadyReplicas,
				UpdatedReplicas: dep.Status.UpdatedReplicas,
				Image:           getMainContainerImage(&dep),
				CreatedAt:       dep.CreationTimestamp.Time,
			}, nil
		}
		
		// Check if deployment selector matches service selector
		if service.Spec.Selector != nil && dep.Spec.Selector != nil {
			match := true
			for key, value := range service.Spec.Selector {
				if depValue, ok := dep.Spec.Selector.MatchLabels[key]; !ok || depValue != value {
					match = false
					break
				}
			}
			if match {
				var replicas int32 = 0
				if dep.Spec.Replicas != nil {
					replicas = *dep.Spec.Replicas
				}
				return &DeploymentInfo{
					Name:            dep.Name,
					Replicas:        replicas,
					ReadyReplicas:   dep.Status.ReadyReplicas,
					UpdatedReplicas: dep.Status.UpdatedReplicas,
					Image:           getMainContainerImage(&dep),
					CreatedAt:       dep.CreationTimestamp.Time,
				}, nil
			}
		}
	}

	// Also try StatefulSets if no deployment found
	statefulsets, err := c.clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, sts := range statefulsets.Items {
			if sts.Name == serviceName {
				var replicas int32 = 0
				if sts.Spec.Replicas != nil {
					replicas = *sts.Spec.Replicas
				}
				return &DeploymentInfo{
					Name:            sts.Name + " (StatefulSet)",
					Replicas:        replicas,
					ReadyReplicas:   sts.Status.ReadyReplicas,
					UpdatedReplicas: sts.Status.UpdatedReplicas,
					Image:           getMainContainerImageFromStatefulSet(&sts),
					CreatedAt:       sts.CreationTimestamp.Time,
				}, nil
			}
		}
	}

	// Also try DaemonSets
	daemonsets, err := c.clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, ds := range daemonsets.Items {
			if ds.Name == serviceName {
				return &DeploymentInfo{
					Name:            ds.Name + " (DaemonSet)",
					Replicas:        ds.Status.DesiredNumberScheduled,
					ReadyReplicas:   ds.Status.NumberReady,
					UpdatedReplicas: ds.Status.UpdatedNumberScheduled,
					Image:           getMainContainerImageFromDaemonSet(&ds),
					CreatedAt:       ds.CreationTimestamp.Time,
				}, nil
			}
		}
	}

	return nil, nil // No deployment found
}

func getMainContainerImageFromStatefulSet(sts *appsv1.StatefulSet) string {
	if len(sts.Spec.Template.Spec.Containers) > 0 {
		return sts.Spec.Template.Spec.Containers[0].Image
	}
	return "unknown"
}

func getMainContainerImageFromDaemonSet(ds *appsv1.DaemonSet) string {
	if len(ds.Spec.Template.Spec.Containers) > 0 {
		return ds.Spec.Template.Spec.Containers[0].Image
	}
	return "unknown"
}

func getMainContainerImage(dep *appsv1.Deployment) string {
	if len(dep.Spec.Template.Spec.Containers) > 0 {
		return dep.Spec.Template.Spec.Containers[0].Image
	}
	return "unknown"
}

func (c *Client) GetClientset() *kubernetes.Clientset {
	return c.clientset
}

func (c *Client) GetConfig() *rest.Config {
	return c.config
}
