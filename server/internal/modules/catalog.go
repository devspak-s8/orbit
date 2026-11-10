package modules

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
)

//go:embed catalog/manifests/*.json
var manifestFS embed.FS

// Catalog is the first-party module catalog: manifests embedded in the
// server binary. Discovery for the alpha means reading this catalog and
// seeding it into the database.
type Catalog struct {
	modules map[string]Manifest
	order   []string
}

// LoadCatalog parses and validates every embedded manifest, failing at
// startup on any invalid module — a bad manifest must never ship silently.
func LoadCatalog() (*Catalog, error) {
	c := &Catalog{modules: map[string]Manifest{}}
	entries, err := fs.Glob(manifestFS, "catalog/manifests/*.json")
	if err != nil {
		return nil, fmt.Errorf("glob manifests: %w", err)
	}
	for _, path := range entries {
		raw, err := manifestFS.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read manifest %s: %w", path, err)
		}
		var m Manifest
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("decode manifest %s: %w", path, err)
		}
		if err := m.Validate(); err != nil {
			return nil, err
		}
		if _, dup := c.modules[m.ID]; dup {
			return nil, fmt.Errorf("duplicate module id %q", m.ID)
		}
		c.modules[m.ID] = m
		c.order = append(c.order, m.ID)
	}
	return c, nil
}

// Get returns a manifest from the catalog.
func (c *Catalog) Get(id string) (Manifest, bool) {
	m, ok := c.modules[id]
	return m, ok
}

// IDs returns catalog module ids in load order.
func (c *Catalog) IDs() []string {
	return append([]string(nil), c.order...)
}
