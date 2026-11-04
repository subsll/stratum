package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/subsll/stratum/internal/model"
)

// RegistryEntry points at one known workspace. The registry is a convenience
// index, never evidence: a case remains valid if the registry is lost.
type RegistryEntry struct {
	Path    string `json:"path"`
	CaseID  string `json:"case_id"`
	Name    string `json:"name"`
	Created string `json:"created"`
}

// Registry is the on-disk list of known workspaces.
type Registry struct {
	FormatVersion int             `json:"format_version"`
	Workspaces    []RegistryEntry `json:"workspaces"`
}

// Home returns STRATUM_HOME or ~/.stratum.
func Home() (string, error) {
	if h := strings.TrimSpace(os.Getenv("STRATUM_HOME")); h != "" {
		abs, err := filepath.Abs(h)
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".stratum"), nil
}

// RegistryPath returns the registry file path, creating its parent directory.
func RegistryPath() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(home, "workspaces.json"), nil
}

// LoadRegistry reads the registry. A missing file is an empty registry.
func LoadRegistry() (*Registry, error) {
	path, err := RegistryPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Registry{FormatVersion: model.FormatVersion}, nil
		}
		return nil, err
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if reg.FormatVersion == 0 {
		reg.FormatVersion = model.FormatVersion
	}
	return &reg, nil
}

// Register adds or updates a workspace entry.
func Register(e RegistryEntry) error {
	if strings.TrimSpace(e.Path) == "" || strings.TrimSpace(e.CaseID) == "" {
		return fmt.Errorf("registry entry needs path and case id")
	}
	abs, err := filepath.Abs(e.Path)
	if err != nil {
		return err
	}
	e.Path = filepath.Clean(abs)

	reg, err := LoadRegistry()
	if err != nil {
		return err
	}
	found := false
	for i := range reg.Workspaces {
		if samePath(reg.Workspaces[i].Path, e.Path) {
			reg.Workspaces[i] = e
			found = true
			break
		}
	}
	if !found {
		reg.Workspaces = append(reg.Workspaces, e)
	}
	sort.SliceStable(reg.Workspaces, func(i, j int) bool {
		if reg.Workspaces[i].Created == reg.Workspaces[j].Created {
			return reg.Workspaces[i].Path < reg.Workspaces[j].Path
		}
		return reg.Workspaces[i].Created > reg.Workspaces[j].Created
	})
	path, err := RegistryPath()
	if err != nil {
		return err
	}
	return writeJSON(path, reg, 0o600)
}

// ListRegistry returns registered workspaces, newest first.
func ListRegistry() ([]RegistryEntry, error) {
	reg, err := LoadRegistry()
	if err != nil {
		return nil, err
	}
	out := append([]RegistryEntry(nil), reg.Workspaces...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Created == out[j].Created {
			return out[i].Path < out[j].Path
		}
		return out[i].Created > out[j].Created
	})
	return out, nil
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return filepath.Clean(aa) == filepath.Clean(bb)
}
