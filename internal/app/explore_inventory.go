package app

import (
	"time"

	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func (m *model) exploreRows() ([]string, []kubernetes.Row) {
	e := m.explorer()
	inventory := e.PrepareInventory(time.Now())
	return inventory.Columns, inventory.Rows
}
