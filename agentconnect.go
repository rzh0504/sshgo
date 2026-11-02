package main

import (
	"github.com/egoist/mygo/ui"

	"sshgo/internal/agent"
	"sshgo/internal/store"
	"sshgo/locales"
)

// agentQueuedSend is a prompt waiting for its host to connect.
type agentQueuedSend struct {
	hostID      string
	text        string
	attachments []agent.Attachment
}

type agentConnectState struct {
	open        bool
	host        agent.Host
	text        string
	attachments []agent.Attachment
}

// beginAgentSend connects a host the prompt runs on, then sends the prompt
// once the host is connected.
func (a *App) beginAgentSend(host agent.Host, text string, attachments []agent.Attachment) {
	h, ok := a.state.Host(host.ID)
	if !ok {
		return
	}
	if a.agentHostConnected(h.ID) {
		a.startAgentTurn(text, attachments)
		return
	}
	a.agent.mu.Lock()
	if a.agent.busy {
		a.agent.mu.Unlock()
		return
	}
	a.agent.queued = &agentQueuedSend{hostID: h.ID, text: text, attachments: attachments}
	a.agent.busy = true
	a.agent.err = ""
	a.agent.mu.Unlock()
	a.repaint()
	a.connectAgentHost(h)
}

func (a *App) connectAgentHost(h store.Host) {
	rt := a.runtime(h.ID)
	switch rt.state {
	case connConnecting:
		return
	case connConnected:
		a.agentQueuedReady(h.ID)
		return
	}
	switch {
	case h.UsesAgent():
		a.connectAgent(h)
	case h.UsesKey():
		a.connectKey(h, rt)
	default:
		a.connectPassword(h, rt)
	}
}

// agentQueuedReady sends the prompt that waited for its host.
func (a *App) agentQueuedReady(hostID string) {
	a.agent.mu.Lock()
	q := a.agent.queued
	if q == nil || q.hostID != hostID {
		a.agent.mu.Unlock()
		return
	}
	a.agent.queued = nil
	a.agent.busy = false
	a.agent.mu.Unlock()
	a.startAgentTurn(q.text, q.attachments)
}

// agentQueuedFailed drops the prompt that waited for its host, if that
// prompt is waiting on hostID.
func (a *App) agentQueuedFailed(hostID, reason string) {
	a.agent.mu.Lock()
	q := a.agent.queued
	if q == nil || q.hostID != hostID {
		a.agent.mu.Unlock()
		return
	}
	a.agent.queued = nil
	a.agent.busy = false
	if reason != "" {
		a.agent.err = reason
	}
	a.agent.mu.Unlock()
	a.repaint()
}

func (a *App) agentConnectView(c *ui.Context, pal *palette) {
	k := a.agentConnect
	if k == nil || !k.open {
		return
	}
	connect := false
	ui.DialogBase(c.Key("agent-connect"), &k.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		floating(panel.Width(420).Padding(22).Gap(14).Radius(14).Background(pal.popover), pal).
			Label(locales.AgentConnectTitle())
		dialogHead(c, pal, locales.AgentConnectTitle(), locales.AgentConnectDetail(k.host.Name, k.host.Address))
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if button(c, "agent-connect-cancel", locales.ActionCancel(), "esc", false, pal) || c.Shortcut(0, ui.KeyEscape) {
				k.open = false
			}
			if button(c, "agent-connect-go", locales.AgentConnectSend(), "", true, pal) {
				connect = true
				k.open = false
			}
		})
	})
	if !k.open {
		a.agentConnect = nil
		if connect {
			a.beginAgentSend(k.host, k.text, k.attachments)
		}
	}
}
