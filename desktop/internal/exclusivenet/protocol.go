package exclusivenet

const (
	helperProtocolVersion = 1

	helperCommandApply   = "apply"
	helperCommandDisable = "disable"
	helperCommandPing    = "ping"
	helperCommandExit    = "exit"

	helperErrorAdministratorRequired = "administrator_required"
)

type helperHello struct {
	Protocol int    `json:"protocol"`
	Token    string `json:"token"`
}

type helperRequest struct {
	ID             uint64 `json:"id"`
	Command        string `json:"command"`
	InterfaceIndex int    `json:"interfaceIndex,omitempty"`
}

type helperResponse struct {
	ID             uint64 `json:"id"`
	OK             bool   `json:"ok"`
	Active         bool   `json:"active"`
	InterfaceIndex int    `json:"interfaceIndex,omitempty"`
	Error          string `json:"error,omitempty"`
	ErrorCode      string `json:"errorCode,omitempty"`
}
