package main

import (
	"testing"

	"sshgo/locales"
)

func TestOSKindsHaveIcons(t *testing.T) {
	if len(osKinds) != 16 {
		t.Fatalf("len(osKinds) = %d, want the 16 core systems", len(osKinds))
	}
	ids, names := map[string]bool{}, map[string]bool{}
	for _, k := range osKinds {
		if k.id == "" || k.name == "" {
			t.Fatalf("system = %+v, want an id and a name", k)
		}
		if k.svg == nil {
			t.Errorf("%s has no icon", k.id)
		}
		if ids[k.id] {
			t.Errorf("two systems share the id %q", k.id)
		}
		if names[k.name] {
			t.Errorf("two systems share the name %q", k.name)
		}
		ids[k.id], names[k.name] = true, true
	}
	if k := osKindByID("ubuntu"); k.id != "ubuntu" || k.svg == nil {
		t.Fatalf("osKindByID(ubuntu) = %+v, want its icon", k)
	}
	if k := osKindByID("missing"); k.id != "" || k.icon() != serverIcon {
		t.Fatalf("osKindByID(missing) = %+v, want the server icon", k)
	}
	if k := osKindByName("Debian"); k.id != "debian" {
		t.Fatalf("osKindByName(Debian) = %+v, want debian", k)
	}
	for _, k := range osKinds {
		if got := osKindByName(k.name); got.id != k.id {
			t.Errorf("osKindByName(%q) = %+v, want %s", k.name, got, k.id)
		}
	}
	allNames := osNames()
	if len(allNames) != len(osKinds)+1 || allNames[len(allNames)-1] != locales.OsOther() {
		t.Fatalf("osNames() = %v, want the systems and the other one", allNames)
	}
}
