package main

import (
	"testing"

	"sshgo/internal/store"
)

func TestDashboardSectionsListUngroupedFirst(t *testing.T) {
	a := &App{state: store.State{
		Groups: []store.Group{{ID: "g1", Name: "Work"}},
		Hosts: []store.Host{
			{ID: "a", Name: "a", Host: "a"},
			{ID: "b", Name: "b", Host: "b", Group: "g1"},
			{ID: "c", Name: "c", Host: "c", Group: "g1"},
		},
	}}
	sections := a.dashboardSections()
	if len(sections) != 2 {
		t.Fatalf("sections = %+v, want the ungrouped and one group", sections)
	}
	if sections[0].id != "" || len(sections[0].hosts) != 1 || sections[0].hosts[0].ID != "a" {
		t.Fatalf("first section = %+v, want host a in no group", sections[0])
	}
	if sections[1].id != "g1" || sections[1].name != "Work" || len(sections[1].hosts) != 2 {
		t.Fatalf("second section = %+v, want the two hosts of g1", sections[1])
	}
}

func TestDashboardSectionsWithoutGroups(t *testing.T) {
	hosts := []store.Host{{ID: "a"}, {ID: "b"}}
	a := &App{state: store.State{Hosts: hosts}}
	sections := a.dashboardSections()
	if len(sections) != 1 || sections[0].name != "" || len(sections[0].hosts) != 2 {
		t.Fatalf("sections = %+v, want one section of every host", sections)
	}
}
