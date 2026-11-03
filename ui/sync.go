package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/ui/syncview"
)

// openSync shows the Sync environment page for the connected server,
// reading both sides afresh unless a sync is under way.
func (a *App) openSync() {
	if a.client == nil || a.host.ID == hosts.LocalID {
		return
	}
	a.page = pageSync
	if a.sync != nil && a.sync.Running() {
		return
	}
	a.closeSync()
	a.page = pageSync
	a.sync = syncview.Open(syncview.Options{
		Host:          a.host,
		Remote:        a.client,
		Local:         LocalHost(),
		DaemonCommand: a.opts.DaemonCommand,
		Update:        a.onMain,
		AgentName:     a.agentName,
		Back:          func() { a.page = pageHosts },
	})
}

func (a *App) closeSync() {
	if a.sync != nil {
		a.sync.Close()
		a.sync = nil
	}
	if a.page == pageSync {
		a.page = ""
	}
}

// syncPage is the Sync environment page; its way back to the hosts leaves a
// sync under way running, the page there to come back to.
func (a *App) syncPage(c *ui.Context) {
	a.sync.View(c)
	if a.sync.Closed() {
		a.sync = nil
		a.page = pageHosts
	}
}
