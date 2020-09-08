package pptcp

const (
	// StateDisconnected 连接断开
	StateDisconnected uint32 = iota
	// StateConnecting 连接建立中
	StateConnecting
	// StateReconnecting 正在重新建立连接
	StateReconnecting
	// StateConnected 已经建立的连接
	StateConnected
)
