// =============================================================================
// FILE: pkg/sandbox/network.go
// SOURCE: https://urunc.io/design/ (Network handling section)
// SOURCE: https://archive.fosdem.org/2024/events/attachments/fosdem-2024-3402/...
//
// DOCS STATE:
//   "urunc creates a new tap device tap0_urunc inside the container netns.
//    CNI provides a veth endpoint inside the netns.
//    urunc maps all incoming traffic to the tap interface.
//    urunc maps all outgoing traffic to the veth endpoint."
//
// IMPLEMENTATION:
//   We prepare a CNI bridge configuration that containerd and CNI will use.
//   urunc then internally creates tap0_urunc and sets up TC rules for mapping.
// =============================================================================

package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// CNIBridgeConfig is the standard CNI bridge plugin config used with urunc.
type CNIBridgeConfig struct {
	CNIVersion string `json:"cniVersion"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Bridge     string `json:"bridge"`
	IsGateway  bool   `json:"isGateway"`
	IPMasq     bool   `json:"ipMasq"`
	IPAM       IPAM   `json:"ipam"`
	DNS        DNS    `json:"dns"`
}

type IPAM struct {
	Type   string  `json:"type"`
	Subnet string  `json:"subnet"`
	Routes []Route `json:"routes"`
}

type Route struct {
	Dst string `json:"dst"`
}

type DNS struct {
	Nameservers []string `json:"nameservers"`
}

// WriteCNIConfig writes the CNI config to /etc/cni/net.d/ for containerd.
func WriteCNIConfig() error {
	const cniDir = "/etc/cni/net.d"
	if err := os.MkdirAll(cniDir, 0755); err != nil {
		return err
	}

	cfg := CNIBridgeConfig{
		CNIVersion: "0.4.0",
		Name:       "urunc-bridge",
		Type:       "bridge",
		Bridge:     "cni0",
		IsGateway:  true,
		IPMasq:     true,
		IPAM: IPAM{
			Type:   "host-local",
			Subnet: "10.88.0.0/16",
			Routes: []Route{{Dst: "0.0.0.0/0"}},
		},
		DNS: DNS{
			Nameservers: []string{"8.8.8.8", "8.8.4.4"},
		},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(cniDir, "10-urunc-bridge.conf")
	return os.WriteFile(path, data, 0644)
}

// NetworkPolicy restricts tool network access based on tool capabilities.
// This is OUR implementation; urunc handles the tap and veth mapping internally.
type NetworkPolicy struct {
	AllowedDomains []string
	AllowedPorts   []int
	EnableNetwork  bool
}

// ToCNICapabilities returns CNI firewall capabilities if available.
// Note: domain-level filtering typically requires an additional CNI plugin
// (for example, bandwidth, firewall) or sidecar. We document this gap explicitly.
func (np *NetworkPolicy) ToCNICapabilities() map[string]interface{} {
	caps := map[string]interface{}{
		"portMappings": []map[string]interface{}{},
	}
	if !np.EnableNetwork {
		// No CNI config means isolated namespace with no external route.
		return nil
	}
	for _, port := range np.AllowedPorts {
		caps["portMappings"] = append(caps["portMappings"].([]map[string]interface{}), map[string]interface{}{
			"hostPort":      port,
			"containerPort": port,
			"protocol":      "tcp",
		})
	}
	return caps
}
