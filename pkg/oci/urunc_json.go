// =============================================================================
// FILE: pkg/oci/urunc_json.go
// SOURCE: https://urunc.io/package/ (Annotations section)
//
// DOCS STATE:
//   "Due to the fact that Docker and some high-level container runtimes do not
//    pass the image annotations to the underlying container runtime, urunc can
//    also read the above information from a file inside the container's rootfs.
//    The file should be named urunc.json, it should be placed in the root
//    directory of the container's rootfs and it should have a JSON format with
//    the above information, where the values are base64 encoded."
// =============================================================================

package oci

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// UruncJSON represents the metadata file placed at /urunc.json in the rootfs.
// Every value MUST be base64 encoded as per official docs.
type UruncJSON map[string]string

// NewUruncJSON creates a urunc.json map from annotations, encoding values to base64.
func NewUruncJSON(annotations map[string]string) UruncJSON {
	uj := make(UruncJSON, len(annotations))
	for k, v := range annotations {
		uj[k] = base64.StdEncoding.EncodeToString([]byte(v))
	}
	return uj
}

// Write writes the urunc.json file to the specified rootfs directory.
func (uj UruncJSON) Write(rootfsDir string) error {
	path := filepath.Join(rootfsDir, "urunc.json")
	data, err := json.MarshalIndent(uj, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal urunc.json: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write urunc.json: %w", err)
	}
	return nil
}
