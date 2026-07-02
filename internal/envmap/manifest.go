package envmap

import (
	"fmt"
	"sort"

	"github.com/pelletier/go-toml/v2"
)

// manifestFile is the on-disk .pass-cli.toml shape: an [env] table mapping each
// ENV_NAME to a "service[/field]" reference.
type manifestFile struct {
	Env map[string]string `toml:"env"`
}

// ParseManifest parses a .pass-cli.toml manifest into env mappings. The [env]
// table maps each ENV_NAME to a "service[/field]" reference — never a value, so
// the file is safe to commit. Mappings are returned sorted by env name for
// deterministic output; each name is validated and each reference is split by
// SplitPath (so the slash grammar and colon alias apply here too).
func ParseManifest(data []byte) ([]Mapping, error) {
	var mf manifestFile
	if err := toml.Unmarshal(data, &mf); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}
	if len(mf.Env) == 0 {
		return nil, nil
	}

	names := make([]string, 0, len(mf.Env))
	for name := range mf.Env {
		names = append(names, name)
	}
	sort.Strings(names)

	mappings := make([]Mapping, 0, len(names))
	for _, name := range names {
		if !ValidEnvName(name) {
			return nil, fmt.Errorf("invalid environment variable name %q in manifest", name)
		}
		service, field, filter, err := SplitPath(mf.Env[name])
		if err != nil {
			return nil, fmt.Errorf("manifest entry %q: %w", name, err)
		}
		if service == "" {
			return nil, fmt.Errorf("manifest entry %q has an empty service", name)
		}
		mappings = append(mappings, Mapping{EnvName: name, Service: service, Field: field, Filter: filter})
	}
	return mappings, nil
}
