package settings

type RowKind uint8

const (
	AgentRow RowKind = iota
	MouseRow
	ContextRow
	KubeconfigRow
	SaveWorkspaceRow
	RenameWorkspaceRow
	AddConnectionRow
	ConnectionRow
	CreateWorkspaceRow
	CloseRow
	ProviderRow
	AddProviderRow
	SaveAgentRow
	CancelAgentRow
)

type Row struct {
	Kind                       RowKind
	ID, Label, Detail, Section string
}
