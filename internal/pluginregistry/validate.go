package pluginregistry

import (
	"fmt"
	"strings"

	"github.com/arcgolabs/collectionx/set"
)

type ValidationReport struct {
	Plugins          int
	Versions         int
	ApprovedVersions int
	RejectedVersions int
	Assets           int
	Warnings         []string
}

func ValidateIndex(index *Index) (ValidationReport, error) {
	if index == nil {
		return ValidationReport{}, fmt.Errorf("plugin registry index is nil")
	}
	report := ValidationReport{Plugins: len(index.Items)}
	pluginIDs := set.NewSet[string]()
	namespaces := set.NewSet[string]()
	for _, plugin := range index.Items {
		if pluginIDs.Contains(plugin.ID) {
			return report, fmt.Errorf("plugin %q is duplicated", plugin.ID)
		}
		pluginIDs.Add(plugin.ID)
		if namespaces.Contains(plugin.Namespace) {
			report.Warnings = append(report.Warnings, fmt.Sprintf("namespace %q is used by more than one plugin", plugin.Namespace))
		}
		namespaces.Add(plugin.Namespace)

		versions := set.NewSet[string]()
		approved := 0
		for _, version := range plugin.Versions {
			report.Versions++
			if versions.Contains(version.Version) {
				return report, fmt.Errorf("plugin %q version %q is duplicated", plugin.ID, version.Version)
			}
			versions.Add(version.Version)
			if version.Approved() {
				approved++
				report.ApprovedVersions++
				if len(version.Assets) == 0 {
					return report, fmt.Errorf("plugin %q approved version %q has no assets", plugin.ID, version.Version)
				}
			} else if strings.EqualFold(strings.TrimSpace(version.Status), "rejected") {
				report.RejectedVersions++
			}
			if err := validateVersionAssets(plugin.ID, version); err != nil {
				return report, err
			}
			report.Assets += len(version.Assets)
		}
		if approved == 0 {
			report.Warnings = append(report.Warnings, fmt.Sprintf("plugin %q has no approved versions", plugin.ID))
		}
	}
	return report, nil
}

func validateVersionAssets(pluginID string, version PluginVersion) error {
	targets := set.NewSet[string]()
	for _, asset := range version.Assets {
		if strings.TrimSpace(asset.URL) == "" {
			return fmt.Errorf("plugin %q version %q has an asset without url", pluginID, version.Version)
		}
		format := assetFormat(asset)
		switch format {
		case "zip", "tar", "tar.gz", "tgz", "dir":
		default:
			return fmt.Errorf("plugin %q version %q asset %q has unsupported format %q", pluginID, version.Version, asset.URL, format)
		}
		target := asset.OS + "/" + asset.Arch
		if target == "/" {
			target = "any"
		}
		if targets.Contains(target) {
			return fmt.Errorf("plugin %q version %q has duplicate asset target %s", pluginID, version.Version, target)
		}
		targets.Add(target)
		for _, signature := range asset.Signatures {
			if strings.TrimSpace(signature.KeyID) == "" {
				return fmt.Errorf("plugin %q version %q asset %q has signature without key_id", pluginID, version.Version, asset.URL)
			}
			if strings.TrimSpace(signature.URL) == "" {
				return fmt.Errorf("plugin %q version %q asset %q has signature without url", pluginID, version.Version, asset.URL)
			}
		}
	}
	return nil
}
