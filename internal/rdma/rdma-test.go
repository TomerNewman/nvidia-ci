package rdmatest

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/nvidia-ci/pkg/clients"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	MinBandwidth       = 10.0 // Minimum BW in Gbps
	MinMsgRate         = 0.1  // Minimum MsgRate in Mpps
	ValidLinkTypes     = "Ethernet,IB"
	MacVlanNetworkName = "rdmashared-net"

	RdmaSharedDeviceResourceName = map[string]corev1.ResourceName{
		"ethernet":   "rdma/rdma_shared_device_eth",
		"infiniband": "rdma/rdma_shared_device_ib",
	}
)

// CreateRdmaWorkloadPod create RDMA worker pod.
func CreateRdmaWorkloadPod(name, namespace, withCuda, mode, hostname, device, crName,
	image, linkType, serverIP string) *corev1.Pod {

	var (
		args []string
	)

	if mode == "server" {
		args = []string{"-c", withCuda, "-m", mode, "-n", "net1", "-d", device}
	} else {
		args = []string{"-c", withCuda, "-m", mode, "-n", "net1", "-d", device, "-i", serverIP}
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"k8s.v1.cni.cncf.io/networks": crName,
			},
		},
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{
				"kubernetes.io/hostname": hostname,
			},
			ServiceAccountName: "rdma",
			Containers: []corev1.Container{
				{
					Name:            name,
					Image:           image,
					ImagePullPolicy: corev1.PullAlways,
					Command:         []string{"/root/entrypoint.sh"},
					Args:            args,
					SecurityContext: &corev1.SecurityContext{
						Privileged: boolPtr(true),
						Capabilities: &corev1.Capabilities{
							Add: []corev1.Capability{"IPC_LOCK"},
						},
					},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							RdmaSharedDeviceResourceName[linkType]: resource.MustParse("1"),
						},
						Requests: corev1.ResourceList{
							RdmaSharedDeviceResourceName[linkType]: resource.MustParse("1"),
						},
					},
				},
			},
		},
	}

	return pod
}

func boolPtr(b bool) *bool {
	return &b
}
func ptrInt64(i int64) *int64 {
	return &i
}

// GetMyServerIP retrieve pod interface ip.
func GetMyServerIP(clientset *clients.Settings, podName, podNamespace, podinterface string) (string, error) {
	pod, err := clientset.Pods(podNamespace).Get(context.TODO(), podName, metav1.GetOptions{})
	// pod, err := apiClient.Pods(podNamespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get pod: %v", err)
	}

	// Extract network-status annotation
	networkStatus, ok := pod.Annotations["k8s.v1.cni.cncf.io/network-status"]
	if !ok {
		return "", fmt.Errorf("network status annotation not found")
	}

	// Parse JSON from annotation
	var networkData []map[string]interface{}
	err = json.Unmarshal([]byte(networkStatus), &networkData)
	if err != nil {
		return "", fmt.Errorf("failed to parse network-status annotation: %v", err)
	}

	// Search for the `ipoib` network entry
	for _, net := range networkData {
		if name, exists := net["interface"]; exists && name == podinterface {
			if ips, exists := net["ips"].([]interface{}); exists && len(ips) > 0 {
				return ips[0].(string), nil // Return the first IP found
			}
		}
	}

	return "", fmt.Errorf("ipoib network IP not found")
}

// ParseRdmaOutput parse Rdma logs.
func ParseRdmaOutput(output string) (map[string]string, error) {
	results := make(map[string]string)

	// Regex patterns
	configRegex := regexp.MustCompile(`([\w-\s\*]+):\s+([\w\[\]\/.]+)`)
	//configRegex := regexp.MustCompile(`(?P<key>[\w\s\*]+):\s+(?P<value>[\w\[\]\/.]+)`)
	bwTableRegex := regexp.MustCompile(`\s*(\d+)\s+(\d+)\s+([\d.]+)\s+([\d.]+)\s+([\d.]+)`)

	scanner := bufio.NewScanner(strings.NewReader(output))
	isParsingConfig := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.Contains(line, "RDMA_Write BW Test") {
			isParsingConfig = true
			results["Test_Type"] = "RDMA_Write BW Test"
			continue
		}

		if strings.Contains(line, "---------------------------------------------------------------------------------------") {
			isParsingConfig = false
		}

		// Parse RDMA configuration key-value pairs
		if isParsingConfig {
			matches := configRegex.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) > 2 {
					key := strings.TrimSpace(match[1])
					value := strings.TrimSpace(match[2])
					results[key] = value
				}
			}
		}

		// Match the performance table
		if matches := bwTableRegex.FindStringSubmatch(line); len(matches) > 4 {
			glog.Infof("Matched Bandwidth Table:%v", matches)
			results["Bytes"] = matches[1]
			results["Iterations"] = matches[2]
			results["BW_Peak_Gbps"] = matches[3]
			results["BW_Avg_Gbps"] = matches[4]
			results["MsgRate_Mpps"] = matches[5]
			break // Stop after finding the first occurrence
		}
	}

	// Check for errors
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

func GetPodLogs(clientset *clients.Settings, namespace, podName string) (string, error) {
	req := clientset.Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{})
	// req := apiClient.Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{})
	logStream, err := req.Stream(context.TODO())
	if err != nil {
		return "", fmt.Errorf("error opening log stream: %v", err)
	}
	defer logStream.Close()

	var logs strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := logStream.Read(buf)
		if n > 0 {
			logs.WriteString(string(buf[:n]))
		}
		if err != nil {
			break
		}
	}

	return logs.String(), nil
}

// ValidateRDMAResults basic validation for rdma tests result.
func ValidateRDMAResults(results map[string]string) (bool, error) {
	// Check Test Type
	testType, exists := results["Test_Type"]
	if !exists || testType != "RDMA_Write BW Test" {
		return false, fmt.Errorf("Invalid Test Type: %s", testType)
	}

	// Check Link Type
	linkType, exists := results["Link type"]
	if !exists || !(linkType == "Ethernet" || linkType == "IB") {
		return false, fmt.Errorf("Invalid Link Type: %s (Expected: Ethernet or IB)", linkType)
	}

	// Check Bandwidth
	bwAvg, err := strconv.ParseFloat(results["BW_Avg_Gbps"], 64)
	if err != nil || bwAvg < MinBandwidth {
		return false, fmt.Errorf("Bandwidth too low: %.2f Gbps (Min: %.2f Gbps)", bwAvg, MinBandwidth)
	}

	// Check Message Rate
	msgRate, err := strconv.ParseFloat(results["MsgRate_Mpps"], 64)
	if err != nil || msgRate < MinMsgRate {
		return false, fmt.Errorf("MsgRate too low: %.3f Mpps (Min: %.1f Mpps)", msgRate, MinMsgRate)
	}

	// If everything is valid
	return true, nil
}

// DeleteMofedDeiverNode delete mofed inventory.
func DeleteMofedDeiverNode(clientset *clients.Settings, podName, namespace, nodeName string) (string, error) {

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: podName,
		},
		Spec: corev1.PodSpec{
			HostPID:       true,
			HostNetwork:   true,
			NodeName:      nodeName,
			RestartPolicy: corev1.RestartPolicyNever,
			Volumes: []corev1.Volume{
				{
					Name: "host-root",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/",
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:    "debugger",
					Image:   "busybox",
					Command: []string{"sh", "-c", "rm -rf /host/opt/mofed-container/inventory"},
					SecurityContext: &corev1.SecurityContext{
						Privileged: boolPtr(true),
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "host-root",
							MountPath: "/host",
						},
					},
				},
			},
		},
	}

	_, err := clientset.Pods(namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create pod: %v", err)
	}

	glog.Info("Waiting for pod to complete...")
	watch, err := clientset.Pods(namespace).Watch(context.TODO(), metav1.ListOptions{
		FieldSelector:  "metadata.name=" + podName,
		TimeoutSeconds: ptrInt64(120),
	})
	if err != nil {
		return "", fmt.Errorf("failed to watch pod: %v", err)
	}
	defer watch.Stop()

	completed := false
	var phase corev1.PodPhase

	for event := range watch.ResultChan() {
		p, ok := event.Object.(*corev1.Pod)
		if !ok {
			continue
		}
		phase = p.Status.Phase
		if phase == corev1.PodSucceeded || phase == corev1.PodFailed {
			completed = true
			break
		}
	}

	if !completed {
		return "", fmt.Errorf("timed out waiting for pod to complete")
	}

	logs, err := GetPodLogs(clientset, namespace, podName)
	if err != nil {
		return "", fmt.Errorf("pod completed with phase %s but failed to get logs: %v", phase, err)
	}

	if phase == corev1.PodFailed {
		return "", fmt.Errorf("command failed. Logs:\n%s", logs)
	}

	// Clean up
	err = clientset.Pods(namespace).Delete(context.TODO(), podName, metav1.DeleteOptions{})
	if err != nil {
		return "", fmt.Errorf("failed deleting pod:%s,%v", podName, err)
	}
	return logs, nil
}
