package modules

import "testing"

func TestManifestValidation(t *testing.T) {
	valid := Manifest{
		ID:          "orbit.demo",
		Name:        "Demo",
		Version:     "0.1.0",
		APIVersion:  "1",
		Permissions: []string{"workspace.read"},
		Dependencies: []string{"orbit.core"},
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid manifest rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{"bad id", func(m *Manifest) { m.ID = "Bad ID!" }},
		{"empty name", func(m *Manifest) { m.Name = "  " }},
		{"bad version", func(m *Manifest) { m.Version = "v1" }},
		{"wrong api version", func(m *Manifest) { m.APIVersion = "2" }},
		{"unknown permission", func(m *Manifest) { m.Permissions = []string{"root.everything"} }},
		{"unknown dependency", func(m *Manifest) { m.Dependencies = []string{"some.thirdparty"} }},
		{"bad event type", func(m *Manifest) { m.Events = []string{"Not An Event"} }},
	}
	for _, tc := range cases {
		m := valid
		tc.mutate(&m)
		if err := m.Validate(); err == nil {
			t.Errorf("%s: invalid manifest accepted", tc.name)
		}
	}
}

func TestLoadCatalog(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	m, ok := c.Get("orbit.demo")
	if !ok {
		t.Fatal("orbit.demo missing from catalog")
	}
	if m.Events == nil || len(m.Events) != 1 || m.Events[0] != "demo.record.created" {
		t.Errorf("demo events = %v, want [demo.record.created]", m.Events)
	}
	if m.Entrypoints.Frontend != "frontend" {
		t.Errorf("demo frontend entrypoint = %q, want frontend", m.Entrypoints.Frontend)
	}
}