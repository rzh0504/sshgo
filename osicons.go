package main

import (
	"embed"
	"log"

	"github.com/egoist/mygo/ui"

	"sshgo/internal/store"
	"sshgo/locales"
)

//go:embed assets/os/*.svg
var osFS embed.FS

// osKind is a system a host runs, with the icon that marks it.
type osKind struct {
	id   string
	name string
	file string
	svg  *ui.SVG
}

// osKinds are the systems the editor offers, the icon of its system in
// the colored SVGs of Dashboard Icons.
var osKinds = []osKind{
	{id: "linux", name: "Linux", file: "linux.svg"},
	{id: "ubuntu", name: "Ubuntu", file: "ubuntu-linux.svg"},
	{id: "debian", name: "Debian", file: "debian-linux.svg"},
	{id: "arch", name: "Arch Linux", file: "arch-linux.svg"},
	{id: "fedora", name: "Fedora", file: "fedora.svg"},
	{id: "opensuse", name: "openSUSE", file: "opensuse.svg"},
	{id: "centos", name: "CentOS", file: "centos.svg"},
	{id: "rocky", name: "Rocky Linux", file: "rocky-linux.svg"},
	{id: "alma", name: "AlmaLinux", file: "alma-linux.svg"},
	{id: "rhel", name: "Red Hat Enterprise Linux", file: "redhat-linux.svg"},
	{id: "alpine", name: "Alpine Linux", file: "alpine-linux.svg"},
	{id: "nixos", name: "NixOS", file: "nixos.svg"},
	{id: "gentoo", name: "Gentoo", file: "gentoo-linux.svg"},
	{id: "kali", name: "Kali Linux", file: "kali-linux.svg"},
	{id: "mint", name: "Linux Mint", file: "linux-mint.svg"},
	{id: "manjaro", name: "Manjaro", file: "manjaro-linux.svg"},
}

func init() {
	for i := range osKinds {
		b, err := osFS.ReadFile("assets/os/" + osKinds[i].file)
		if err != nil {
			log.Printf("sshgo: system icon %s: %v", osKinds[i].file, err)
			continue
		}
		s, err := ui.ParseSVG(b)
		if err != nil {
			log.Printf("sshgo: system icon %s: %v", osKinds[i].file, err)
			continue
		}
		osKinds[i].svg = s
	}
}

// osKindByID is the system an id names, the other system for an id that
// names none of them.
func osKindByID(id string) osKind {
	for _, k := range osKinds {
		if k.id == id {
			return k
		}
	}
	return osOtherKind()
}

func osOtherKind() osKind {
	return osKind{name: locales.OsOther()}
}

// osNames are the systems the editor offers, the other system last.
func osNames() []string {
	names := make([]string, 0, len(osKinds)+1)
	for _, k := range osKinds {
		names = append(names, k.name)
	}
	return append(names, osOtherKind().name)
}

// osKindByName is the system a name names, the other system when it names
// none of them.
func osKindByName(name string) osKind {
	for _, k := range osKinds {
		if k.name == name {
			return k
		}
	}
	return osOtherKind()
}

// icon is the system's icon, the server's for the other system.
func (k osKind) icon() *ui.SVG {
	if k.svg != nil {
		return k.svg
	}
	return serverIcon
}

// hostGlyph is the icon that marks a host: its system's, in the system's
// colors, or the server's in the host's color for no system.
func (a *App) hostGlyph(c *ui.Context, pal *palette, h store.Host, size float32) ui.Element {
	k := osKindByID(h.OS)
	img := ui.Image(c, k.icon()).Size(size, size).Shrink(0)
	if k.svg == nil || k.id == "" {
		img.TextColor(hostColor(pal, h.Name))
	}
	return img
}
